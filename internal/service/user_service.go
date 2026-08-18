package service

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/audit"
	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/query"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// UserService implements zerxv1connect.UserServiceHandler. All write operations
// require the admin role.
type UserService struct {
	db     *gorm.DB
	policy *auth.Policy
	media  *media.Media
}

var _ zerxv1connect.UserServiceHandler = (*UserService)(nil)

// NewUserService constructs the user handler.
func NewUserService(db *gorm.DB, policy *auth.Policy, m *media.Media) *UserService {
	return &UserService{db: db, policy: policy, media: m}
}

// ListUsers returns a page of users, or name-matched users when a keyword is
// supplied (demonstrating the GORM-CLI generated query).
func (s *UserService) ListUsers(ctx context.Context, req *connect.Request[zerxv1.ListUsersRequest]) (*connect.Response[zerxv1.ListUsersResponse], error) {
	if keyword := req.Msg.GetKeyword(); keyword != "" {
		users, err := query.Query[model.User](s.db).SearchByName(ctx, "%"+keyword+"%")
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		out, err := s.enrichUsers(ctx, users)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}

		return connect.NewResponse(&zerxv1.ListUsersResponse{
			Users: out,
			Total: int64(len(users)),
		}), nil
	}

	page := int(req.Msg.GetPage().GetPage())
	if page < 1 {
		page = 1
	}
	pageSize := int(req.Msg.GetPage().GetPageSize())
	if pageSize < 1 || pageSize > maxPageSize {
		pageSize = defaultPageSize
	}

	total, err := gorm.G[model.User](s.db).Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	users, err := gorm.G[model.User](s.db).
		Order("id ASC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	out, err := s.enrichUsers(ctx, users)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.ListUsersResponse{
		Users: out,
		Total: total,
	}), nil
}

// GetUser returns a single user by id.
func (s *UserService) GetUser(ctx context.Context, req *connect.Request[zerxv1.GetUserRequest]) (*connect.Response[zerxv1.User], error) {
	u, err := gorm.G[model.User](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	roles, err := userRoleCodes(ctx, s.db, u.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	totpOn, err := userTOTPEnabled(ctx, s.db, u.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(toProtoUser(u, roles, totpOn, s.media)), nil
}

// CreateUser creates a user. Authorization is enforced by the Casbin interceptor.
func (s *UserService) CreateUser(ctx context.Context, req *connect.Request[zerxv1.CreateUserRequest]) (*connect.Response[zerxv1.User], error) {
	_, err := gorm.G[model.User](s.db).Where("email = ?", req.Msg.GetEmail()).First(ctx)
	switch {
	case err == nil:
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("email already in use"))
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	roles := unique(req.Msg.GetRoles())
	if ok, err := rolesExist(ctx, s.db, roles); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	} else if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("包含不存在的角色"))
	}

	if err := s.policy.Validate(req.Msg.GetPassword()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	hash, err := auth.Hash(req.Msg.GetPassword())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	u := model.User{
		Email:        req.Msg.GetEmail(),
		Name:         req.Msg.GetName(),
		Nickname:     req.Msg.GetNickname(),
		Phone:        req.Msg.GetPhone(),
		PasswordHash: hash,
		Status:       true,
	}
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		ur := make([]model.UserRole, 0, len(roles))
		for _, r := range roles {
			ur = append(ur, model.UserRole{UserID: u.ID, RoleCode: r})
		}
		return tx.CreateInBatches(&ur, 100).Error
	})
	if txErr != nil {
		return nil, connect.NewError(connect.CodeInternal, txErr)
	}
	_ = s.policy.RecordHistory(ctx, s.db, u.ID, hash)
	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"id": u.ID, "email": u.Email, "name": u.Name, "roles": roles}}))

	return connect.NewResponse(toProtoUser(u, roles, false, s.media)), nil
}

// UpdateUser updates a user's profile, role, and status. Authorization is
// enforced by the Casbin interceptor.
func (s *UserService) UpdateUser(ctx context.Context, req *connect.Request[zerxv1.UpdateUserRequest]) (*connect.Response[zerxv1.User], error) {
	id := req.Msg.GetId()
	before, err := gorm.G[model.User](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	beforeRoles, err := userRoleCodes(ctx, s.db, id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	roles := unique(req.Msg.GetRoles())
	if ok, err := rolesExist(ctx, s.db, roles); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	} else if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("包含不存在的角色"))
	}

	// Select the columns explicitly so the boolean status (which may be false)
	// is persisted; other empty strings simply overwrite with the new value.
	updates := map[string]any{
		"name":     req.Msg.GetName(),
		"nickname": req.Msg.GetNickname(),
		"phone":    req.Msg.GetPhone(),
		"status":   req.Msg.GetStatus(),
	}
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		ur := make([]model.UserRole, 0, len(roles))
		for _, r := range roles {
			ur = append(ur, model.UserRole{UserID: id, RoleCode: r})
		}
		return tx.CreateInBatches(&ur, 100).Error
	})
	if txErr != nil {
		return nil, connect.NewError(connect.CodeInternal, txErr)
	}

	u, err := gorm.G[model.User](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	totpOn, _ := userTOTPEnabled(ctx, s.db, id)
	audit.Record(ctx, auditJSON(map[string]any{
		"before": map[string]any{"name": before.Name, "nickname": before.Nickname, "phone": before.Phone, "status": before.Status, "roles": beforeRoles},
		"after":  map[string]any{"name": u.Name, "nickname": u.Nickname, "phone": u.Phone, "status": u.Status, "roles": roles},
	}))

	return connect.NewResponse(toProtoUser(u, roles, totpOn, s.media)), nil
}

// DeleteUser soft-deletes a user. Authorization is enforced by the Casbin
// interceptor.
func (s *UserService) DeleteUser(ctx context.Context, req *connect.Request[zerxv1.DeleteUserRequest]) (*connect.Response[zerxv1.DeleteUserResponse], error) {
	id := req.Msg.GetId()
	before, _ := gorm.G[model.User](s.db).Where("id = ?", id).First(ctx)
	rows, err := gorm.G[model.User](s.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"id": before.ID, "email": before.Email, "name": before.Name}}))

	return connect.NewResponse(&zerxv1.DeleteUserResponse{}), nil
}

// ResetPassword sets a user's password and revokes all their sessions so the
// change takes effect immediately. Authorization is enforced by the Casbin
// interceptor.
func (s *UserService) ResetPassword(ctx context.Context, req *connect.Request[zerxv1.ResetPasswordRequest]) (*connect.Response[zerxv1.ResetPasswordResponse], error) {
	id := req.Msg.GetId()
	if _, err := gorm.G[model.User](s.db).Where("id = ?", id).First(ctx); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if err := s.policy.Validate(req.Msg.GetPassword()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := s.policy.CheckHistory(ctx, s.db, id, req.Msg.GetPassword()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	hash, err := auth.Hash(req.Msg.GetPassword())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if err := s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Update("password_hash", hash).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	_ = s.policy.RecordHistory(ctx, s.db, id, hash)
	// Revoke the user's sessions so existing refresh tokens stop working.
	if err := s.db.WithContext(ctx).Where("user_id = ?", id).Delete(&model.UserSession{}).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.ResetPasswordResponse{}), nil
}

// DisableUserTotp force-disables a user's 2FA. Authorization is enforced by the
// Casbin interceptor.
func (s *UserService) DisableUserTotp(ctx context.Context, req *connect.Request[zerxv1.DisableUserTotpRequest]) (*connect.Response[zerxv1.DisableUserTotpResponse], error) {
	id := req.Msg.GetId()
	if err := s.db.WithContext(ctx).Where("user_id = ?", id).Delete(&model.UserTOTP{}).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.db.WithContext(ctx).Where("user_id = ?", id).Delete(&model.TOTPRecoveryCode{}).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"user_id": id, "totp_enabled": false}}))

	return connect.NewResponse(&zerxv1.DisableUserTotpResponse{}), nil
}

// enrichUsers builds proto users with roles and TOTP state in batch.
func (s *UserService) enrichUsers(ctx context.Context, users []model.User) ([]*zerxv1.User, error) {
	ids := make([]uint64, 0, len(users))
	for i := range users {
		ids = append(ids, users[i].ID)
	}
	rolesByUser, err := rolesByUserIDs(ctx, s.db, ids)
	if err != nil {
		return nil, err
	}
	totpByUser, err := totpEnabledByUserIDs(ctx, s.db, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*zerxv1.User, 0, len(users))
	for i := range users {
		out = append(out, toProtoUser(users[i], rolesByUser[users[i].ID], totpByUser[users[i].ID], s.media))
	}

	return out, nil
}
