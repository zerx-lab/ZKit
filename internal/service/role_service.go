package service

import (
	"connectrpc.com/connect"
	"context"
	"errors"
	casbinpkg "github.com/casbin/casbin/v3"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/audit"
	"github.com/zerx-lab/zkit/internal/casbin"
	"github.com/zerx-lab/zkit/internal/model"
)

// RoleService implements zerxv1connect.RoleServiceHandler. Authorization is
// enforced by the Casbin interceptor.
type RoleService struct {
	db       *gorm.DB
	enforcer *casbinpkg.SyncedCachedEnforcer
}

var _ zerxv1connect.RoleServiceHandler = (*RoleService)(nil)

// NewRoleService constructs the role handler.
func NewRoleService(db *gorm.DB, enforcer *casbinpkg.SyncedCachedEnforcer) *RoleService {
	return &RoleService{db: db, enforcer: enforcer}
}

func (s *RoleService) ListRoles(ctx context.Context, _ *connect.Request[zerxv1.ListRolesRequest]) (*connect.Response[zerxv1.ListRolesResponse], error) {
	roles, err := gorm.G[model.Role](s.db).Order("sort ASC, id ASC").Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	out := make([]*zerxv1.Role, 0, len(roles))
	for i := range roles {
		out = append(out, toProtoRole(roles[i]))
	}

	return connect.NewResponse(&zerxv1.ListRolesResponse{Roles: out}), nil
}

func (s *RoleService) CreateRole(ctx context.Context, req *connect.Request[zerxv1.CreateRoleRequest]) (*connect.Response[zerxv1.Role], error) {
	_, err := gorm.G[model.Role](s.db).Where("code = ?", req.Msg.GetCode()).First(ctx)
	switch {
	case err == nil:
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("role code already in use"))
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	r := model.Role{
		Code:        req.Msg.GetCode(),
		Name:        req.Msg.GetName(),
		Description: req.Msg.GetDescription(),
		Sort:        int(req.Msg.GetSort()),
	}
	if err := gorm.G[model.Role](s.db).Create(ctx, &r); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"code": r.Code, "name": r.Name}}))

	return connect.NewResponse(toProtoRole(r)), nil
}

func (s *RoleService) UpdateRole(ctx context.Context, req *connect.Request[zerxv1.UpdateRoleRequest]) (*connect.Response[zerxv1.Role], error) {
	id := req.Msg.GetId()
	if _, err := gorm.G[model.Role](s.db).Where("id = ?", id).First(ctx); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("role not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Code is immutable; only name/description/sort change.
	updates := map[string]any{
		"name":        req.Msg.GetName(),
		"description": req.Msg.GetDescription(),
		"sort":        int(req.Msg.GetSort()),
	}
	if err := s.db.WithContext(ctx).Model(&model.Role{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	r, err := gorm.G[model.Role](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"id": r.ID, "name": r.Name, "description": r.Description}}))

	return connect.NewResponse(toProtoRole(r)), nil
}

func (s *RoleService) DeleteRole(ctx context.Context, req *connect.Request[zerxv1.DeleteRoleRequest]) (*connect.Response[zerxv1.DeleteRoleResponse], error) {
	r, err := gorm.G[model.Role](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("role not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if r.Builtin {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("内置角色不可删除"))
	}

	inUse, err := gorm.G[model.UserRole](s.db).Where("role_code = ?", r.Code).Count(ctx, "user_id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if inUse > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("角色仍被用户使用，无法删除"))
	}

	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", r.ID).Delete(&model.Role{}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_code = ?", r.Code).Delete(&model.RoleMenu{}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_code = ?", r.Code).Delete(&model.RoleButton{}).Error; err != nil {
			return err
		}

		return nil
	})
	if txErr != nil {
		return nil, connect.NewError(connect.CodeInternal, txErr)
	}

	if err := casbin.RemoveRole(s.enforcer, r.Code); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"code": r.Code, "name": r.Name}}))

	return connect.NewResponse(&zerxv1.DeleteRoleResponse{}), nil
}

func (s *RoleService) GetRolePermissions(ctx context.Context, req *connect.Request[zerxv1.GetRolePermissionsRequest]) (*connect.Response[zerxv1.GetRolePermissionsResponse], error) {
	code := req.Msg.GetRoleCode()

	roleMenus, err := gorm.G[model.RoleMenu](s.db).Where("role_code = ?", code).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	menuIDs := make([]uint64, 0, len(roleMenus))
	for i := range roleMenus {
		menuIDs = append(menuIDs, roleMenus[i].MenuID)
	}

	roleButtons, err := gorm.G[model.RoleButton](s.db).Where("role_code = ?", code).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	buttonIDs := make([]uint64, 0, len(roleButtons))
	for i := range roleButtons {
		buttonIDs = append(buttonIDs, roleButtons[i].ButtonID)
	}

	procs, err := casbin.GetRoleProcedures(s.enforcer, code)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.GetRolePermissionsResponse{
		MenuIds:    menuIDs,
		Procedures: procs,
		ButtonIds:  buttonIDs,
	}), nil
}

func (s *RoleService) SetRolePermissions(ctx context.Context, req *connect.Request[zerxv1.SetRolePermissionsRequest]) (*connect.Response[zerxv1.SetRolePermissionsResponse], error) {
	code := req.Msg.GetRoleCode()

	if _, err := gorm.G[model.Role](s.db).Where("code = ?", code).First(ctx); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("role not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_code = ?", code).Delete(&model.RoleMenu{}).Error; err != nil {
			return err
		}
		if menuIDs := req.Msg.GetMenuIds(); len(menuIDs) > 0 {
			rows := make([]model.RoleMenu, 0, len(menuIDs))
			for _, id := range menuIDs {
				rows = append(rows, model.RoleMenu{RoleCode: code, MenuID: id})
			}
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}

		if err := tx.Where("role_code = ?", code).Delete(&model.RoleButton{}).Error; err != nil {
			return err
		}
		if buttonIDs := req.Msg.GetButtonIds(); len(buttonIDs) > 0 {
			rows := make([]model.RoleButton, 0, len(buttonIDs))
			for _, id := range buttonIDs {
				rows = append(rows, model.RoleButton{RoleCode: code, ButtonID: id})
			}
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if txErr != nil {
		return nil, connect.NewError(connect.CodeInternal, txErr)
	}

	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"role_code": code, "menu_ids": req.Msg.GetMenuIds(), "button_ids": req.Msg.GetButtonIds(), "procedures": req.Msg.GetProcedures()}}))
	if err := casbin.SetRoleProcedures(s.enforcer, code, req.Msg.GetProcedures()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.SetRolePermissionsResponse{}), nil
}
