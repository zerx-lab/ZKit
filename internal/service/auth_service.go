package service

import (
	"context"
	"errors"
	"net"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/captcha"
	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/mailer"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/param"
	"github.com/zerx-lab/zkit/internal/ratelimit"
)

// AuthService implements zerxv1connect.AuthServiceHandler.
type AuthService struct {
	db      *gorm.DB
	issuer  *auth.Issuer
	guard   *ratelimit.LoginGuard
	captcha *captcha.Manager
	cfg     config.AuthConfig
	mailer  *mailer.Mailer
	policy  *auth.Policy
	param   *param.Cache
	media   *media.Media
}

var _ zerxv1connect.AuthServiceHandler = (*AuthService)(nil)

// NewAuthService constructs the auth handler.
func NewAuthService(db *gorm.DB, issuer *auth.Issuer, guard *ratelimit.LoginGuard, cap *captcha.Manager, cfg config.AuthConfig, m *mailer.Mailer, policy *auth.Policy, paramCache *param.Cache, mr *media.Media) *AuthService {
	return &AuthService{db: db, issuer: issuer, guard: guard, captcha: cap, cfg: cfg, mailer: m, policy: policy, param: paramCache, media: mr}
}

// clientIP returns the request peer's host portion (port stripped).
func clientIP(req connect.AnyRequest) string {
	addr := req.Peer().Addr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}

	return addr
}

func userAgent(req connect.AnyRequest) string {
	return req.Header().Get("User-Agent")
}

// GetCaptcha issues a fresh captcha challenge (public).
func (s *AuthService) GetCaptcha(_ context.Context, _ *connect.Request[zerxv1.GetCaptchaRequest]) (*connect.Response[zerxv1.GetCaptchaResponse], error) {
	id, b64, err := s.captcha.Generate()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.GetCaptchaResponse{CaptchaId: id, ImageBase64: b64}), nil
}

// Login verifies credentials, enforces brute-force protection, records a login
// log, creates a session, and issues an access + refresh token pair.
func (s *AuthService) Login(ctx context.Context, req *connect.Request[zerxv1.LoginRequest]) (*connect.Response[zerxv1.LoginResponse], error) {
	ip := clientIP(req)
	ua := userAgent(req)
	email := req.Msg.GetEmail()
	key := email + "|" + ip

	if locked, _ := s.guard.Locked(key); locked {
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("账号暂时锁定，请稍后再试"))
	}

	if s.guard.NeedCaptcha(key) {
		if !s.captcha.Verify(req.Msg.GetCaptchaId(), req.Msg.GetCaptchaCode()) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("需要验证码或验证码错误"))
		}
	}

	fail := func(uid uint64, e error) error {
		s.guard.Fail(key)
		s.writeLoginLog(model.LoginLog{UserID: uid, Email: email, IP: ip, UserAgent: ua, Success: false, Error: e.Error()})
		return e
	}

	u, err := gorm.G[model.User](s.db).Where("email = ?", email).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fail(0, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials")))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := auth.Verify(u.PasswordHash, req.Msg.GetPassword()); err != nil {
		return nil, fail(u.ID, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials")))
	}
	if !u.Status {
		return nil, fail(u.ID, connect.NewError(connect.CodePermissionDenied, errors.New("账号已禁用")))
	}

	// 2FA: if enabled, require a valid TOTP or recovery code.
	tt, ttErr := gorm.G[model.UserTOTP](s.db).Where("user_id = ?", u.ID).First(ctx)
	totpOn := ttErr == nil && tt.Enabled
	if totpOn {
		code := req.Msg.GetTotpCode()
		if code == "" {
			// Not a failure: prompt the client for the second factor.
			return connect.NewResponse(&zerxv1.LoginResponse{TotpRequired: true}), nil
		}
		if !totp.Validate(code, tt.Secret) {
			if !s.consumeRecoveryCode(ctx, u.ID, code) {
				return nil, fail(u.ID, connect.NewError(connect.CodeUnauthenticated, errors.New("两步验证码错误")))
			}
		}
	}

	roles, err := userRoleCodes(ctx, s.db, u.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	sid, access, refresh, err := s.startSessionTx(ctx, u, roles, ip, ua)
	if err != nil {
		return nil, err
	}

	s.guard.Reset(key)
	s.writeLoginLog(model.LoginLog{UserID: u.ID, Email: email, IP: ip, UserAgent: ua, Success: true})

	return connect.NewResponse(&zerxv1.LoginResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		User:         toProtoUser(u, roles, totpOn, s.media),
		SessionId:    sid,
	}), nil
}

// consumeRecoveryCode marks a matching unused recovery code used and returns
// whether one matched.
func (s *AuthService) consumeRecoveryCode(ctx context.Context, userID uint64, code string) bool {
	rows, err := gorm.G[model.TOTPRecoveryCode](s.db).Where("user_id = ? AND used_at IS NULL", userID).Find(ctx)
	if err != nil {
		return false
	}
	for i := range rows {
		if auth.Verify(rows[i].CodeHash, code) == nil {
			now := time.Now()
			_ = s.db.WithContext(ctx).Model(&model.TOTPRecoveryCode{}).Where("id = ?", rows[i].ID).Update("used_at", now).Error
			return true
		}
	}
	return false
}

// startSessionTx persists a UserSession (enforcing single-session if configured)
// and mints the access + refresh token pair.
func (s *AuthService) startSessionTx(ctx context.Context, u model.User, roles []string, ip, ua string) (sid, access, refresh string, err error) {
	sid = uuid.NewString()
	now := time.Now()
	session := model.UserSession{
		ID:         sid,
		UserID:     u.ID,
		IP:         ip,
		UserAgent:  ua,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(s.issuer.RefreshTTL()),
	}

	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		if s.cfg.SingleSession {
			if err := tx.Where("user_id = ? AND id <> ?", u.ID, sid).Delete(&model.UserSession{}).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if txErr != nil {
		return "", "", "", connect.NewError(connect.CodeInternal, txErr)
	}

	access, err = s.issuer.IssueAccess(u.ID, roles)
	if err != nil {
		return "", "", "", connect.NewError(connect.CodeInternal, err)
	}
	refresh, err = s.issuer.IssueRefresh(u.ID, sid)
	if err != nil {
		return "", "", "", connect.NewError(connect.CodeInternal, err)
	}

	return sid, access, refresh, nil
}

// Register creates an account. The first user to register becomes admin; all
// subsequent self-registrations are regular users.
func (s *AuthService) Register(ctx context.Context, req *connect.Request[zerxv1.RegisterRequest]) (*connect.Response[zerxv1.RegisterResponse], error) {
	count, err := gorm.G[model.User](s.db).Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	_, err = gorm.G[model.User](s.db).Where("email = ?", req.Msg.GetEmail()).First(ctx)
	switch {
	case err == nil:
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("email already in use"))
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if err := s.policy.Validate(req.Msg.GetPassword()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	hash, err := auth.Hash(req.Msg.GetPassword())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	roleCode := model.RoleUser
	if count == 0 {
		roleCode = model.RoleAdmin
	}

	u := model.User{
		Email:        req.Msg.GetEmail(),
		Name:         req.Msg.GetName(),
		PasswordHash: hash,
		Status:       true,
	}
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		return tx.Create(&model.UserRole{UserID: u.ID, RoleCode: roleCode}).Error
	})
	if txErr != nil {
		return nil, connect.NewError(connect.CodeInternal, txErr)
	}
	_ = s.policy.RecordHistory(ctx, s.db, u.ID, hash)

	roles := []string{roleCode}
	sid, access, refresh, err := s.startSessionTx(ctx, u, roles, clientIP(req), userAgent(req))
	if err != nil {
		return nil, err
	}

	s.writeLoginLog(model.LoginLog{UserID: u.ID, Email: u.Email, Success: true})

	return connect.NewResponse(&zerxv1.RegisterResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		User:         toProtoUser(u, roles, false, s.media),
		SessionId:    sid,
	}), nil
}

// Refresh validates the refresh token, checks the session is still live, and
// issues a fresh access token (re-reading the user so role changes propagate).
func (s *AuthService) Refresh(ctx context.Context, req *connect.Request[zerxv1.RefreshRequest]) (*connect.Response[zerxv1.RefreshResponse], error) {
	claims, err := s.issuer.ParseRefresh(req.Msg.GetRefreshToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid refresh token"))
	}

	session, err := gorm.G[model.UserSession](s.db).Where("id = ?", claims.ID).First(ctx)
	if err != nil || session.ExpiresAt.Before(time.Now()) {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("会话已失效"))
	}

	if _, err := gorm.G[model.UserSession](s.db).Where("id = ?", claims.ID).Update(ctx, "last_seen_at", time.Now()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	u, err := gorm.G[model.User](s.db).Where("id = ?", claims.UserID).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("user not found"))
	}

	roles, err := userRoleCodes(ctx, s.db, u.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	access, err := s.issuer.IssueAccess(u.ID, roles)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.RefreshResponse{AccessToken: access}), nil
}

// Logout revokes every session of the current user.
func (s *AuthService) Logout(ctx context.Context, _ *connect.Request[zerxv1.LogoutRequest]) (*connect.Response[zerxv1.LogoutResponse], error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	if err := s.db.WithContext(ctx).Where("user_id = ?", claims.UserID).Delete(&model.UserSession{}).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.LogoutResponse{}), nil
}

// Me returns the currently authenticated user.
func (s *AuthService) Me(ctx context.Context, _ *connect.Request[zerxv1.MeRequest]) (*connect.Response[zerxv1.MeResponse], error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	u, err := gorm.G[model.User](s.db).Where("id = ?", claims.UserID).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	roles, err := userRoleCodes(ctx, s.db, u.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	totpOn, _ := userTOTPEnabled(ctx, s.db, u.ID)

	return connect.NewResponse(&zerxv1.MeResponse{User: toProtoUser(u, roles, totpOn, s.media)}), nil
}

// ListSessions returns the caller's sessions; admins may target another user.
func (s *AuthService) ListSessions(ctx context.Context, req *connect.Request[zerxv1.ListSessionsRequest]) (*connect.Response[zerxv1.ListSessionsResponse], error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	target := req.Msg.GetUserId()
	if target == 0 {
		target = claims.UserID
	} else if target != claims.UserID && !slices.Contains(claims.Roles, model.RoleAdmin) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("forbidden"))
	}

	sessions, err := gorm.G[model.UserSession](s.db).Where("user_id = ?", target).Order("last_seen_at DESC").Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	out := make([]*zerxv1.Session, 0, len(sessions))
	for i := range sessions {
		out = append(out, toProtoSession(sessions[i]))
	}

	return connect.NewResponse(&zerxv1.ListSessionsResponse{Sessions: out}), nil
}

// RevokeSession deletes a session owned by the caller, or any session for admins.
func (s *AuthService) RevokeSession(ctx context.Context, req *connect.Request[zerxv1.RevokeSessionRequest]) (*connect.Response[zerxv1.RevokeSessionResponse], error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	session, err := gorm.G[model.UserSession](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("session not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if session.UserID != claims.UserID && !slices.Contains(claims.Roles, model.RoleAdmin) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("forbidden"))
	}

	if _, err := gorm.G[model.UserSession](s.db).Where("id = ?", req.Msg.GetId()).Delete(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.RevokeSessionResponse{}), nil
}

func (s *AuthService) writeLoginLog(rec model.LoginLog) {
	rec.CreatedAt = time.Now()
	db := s.db
	go func() {
		_ = gorm.G[model.LoginLog](db).Create(context.Background(), &rec)
	}()
}
