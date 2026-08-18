package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/model"
)

// ChangePassword changes the caller's own password and revokes their sessions.
func (s *AuthService) ChangePassword(ctx context.Context, req *connect.Request[zerxv1.ChangePasswordRequest]) (*connect.Response[zerxv1.ChangePasswordResponse], error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	u, err := gorm.G[model.User](s.db).Where("id = ?", claims.UserID).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	if err := auth.Verify(u.PasswordHash, req.Msg.GetOldPassword()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("原密码错误"))
	}
	if err := s.policy.Validate(req.Msg.GetNewPassword()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := s.policy.CheckHistory(ctx, s.db, u.ID, req.Msg.GetNewPassword()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	hash, err := auth.Hash(req.Msg.GetNewPassword())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", u.ID).Update("password_hash", hash).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	_ = s.policy.RecordHistory(ctx, s.db, u.ID, hash)
	if err := s.db.WithContext(ctx).Where("user_id = ?", u.ID).Delete(&model.UserSession{}).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.ChangePasswordResponse{}), nil
}

// UpdateProfile updates the caller's own profile fields.
func (s *AuthService) UpdateProfile(ctx context.Context, req *connect.Request[zerxv1.UpdateProfileRequest]) (*connect.Response[zerxv1.User], error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	updates := map[string]any{
		"nickname": req.Msg.GetNickname(),
		"avatar":   s.media.NormalizeStored(req.Msg.GetAvatar()),
		"phone":    req.Msg.GetPhone(),
	}
	if err := s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", claims.UserID).Updates(updates).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	u, err := gorm.G[model.User](s.db).Where("id = ?", claims.UserID).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	roles, err := userRoleCodes(ctx, s.db, u.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	totpOn, _ := userTOTPEnabled(ctx, s.db, u.ID)

	return connect.NewResponse(toProtoUser(u, roles, totpOn, s.media)), nil
}

// RequestPasswordReset emails a reset link. To avoid account enumeration it
// always reports success regardless of whether the email exists.
func (s *AuthService) RequestPasswordReset(ctx context.Context, req *connect.Request[zerxv1.RequestPasswordResetRequest]) (*connect.Response[zerxv1.RequestPasswordResetResponse], error) {
	email := req.Msg.GetEmail()
	u, err := gorm.G[model.User](s.db).Where("email = ?", email).First(ctx)
	if err != nil {
		return connect.NewResponse(&zerxv1.RequestPasswordResetResponse{}), nil //nolint:nilerr // enumeration guard
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	rec := model.PasswordResetToken{
		TokenHash: hex.EncodeToString(sum[:]),
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	if err := s.db.WithContext(ctx).Create(&rec).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	domain, _ := s.param.Get(siteDomainKey)
	link := domain + "/reset-password?token=" + token
	body := fmt.Sprintf(`<p>您正在重置密码。请点击以下链接（30 分钟内有效）：</p><p><a href="%s">%s</a></p>`, link, link)
	_ = s.mailer.Send(ctx, email, "重置密码", body)

	return connect.NewResponse(&zerxv1.RequestPasswordResetResponse{}), nil
}

// ConfirmPasswordReset sets a new password using an emailed token.
func (s *AuthService) ConfirmPasswordReset(ctx context.Context, req *connect.Request[zerxv1.ConfirmPasswordResetRequest]) (*connect.Response[zerxv1.ConfirmPasswordResetResponse], error) {
	sum := sha256.Sum256([]byte(req.Msg.GetToken()))
	hashHex := hex.EncodeToString(sum[:])
	rec, err := gorm.G[model.PasswordResetToken](s.db).
		Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", hashHex, time.Now()).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("重置链接无效或已过期"))
	}
	if err := s.policy.Validate(req.Msg.GetNewPassword()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := s.policy.CheckHistory(ctx, s.db, rec.UserID, req.Msg.GetNewPassword()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	hash, err := auth.Hash(req.Msg.GetNewPassword())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	now := time.Now()
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("id = ?", rec.UserID).Update("password_hash", hash).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.PasswordResetToken{}).Where("id = ?", rec.ID).Update("used_at", now).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", rec.UserID).Delete(&model.UserSession{}).Error
	})
	if txErr != nil {
		return nil, connect.NewError(connect.CodeInternal, txErr)
	}
	_ = s.policy.RecordHistory(ctx, s.db, rec.UserID, hash)

	return connect.NewResponse(&zerxv1.ConfirmPasswordResetResponse{}), nil
}

// SetupTotp begins 2FA enrollment for the caller, returning a secret and QR.
func (s *AuthService) SetupTotp(ctx context.Context, _ *connect.Request[zerxv1.SetupTotpRequest]) (*connect.Response[zerxv1.SetupTotpResponse], error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	u, err := gorm.G[model.User](s.db).Where("id = ?", claims.UserID).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	issuer, _ := s.param.Get(siteNameKey)
	if issuer == "" {
		issuer = "ZKit"
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: issuer, AccountName: u.Email})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.db.WithContext(ctx).Clauses(onConflictUserTOTP()).
		Create(&model.UserTOTP{UserID: u.ID, Secret: key.Secret(), Enabled: false}).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	img, err := key.Image(256, 256)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	var buf []byte
	buf, err = encodePNG(img)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf)

	return connect.NewResponse(&zerxv1.SetupTotpResponse{Secret: key.Secret(), QrImageBase64: dataURL}), nil
}

// ActivateTotp confirms 2FA enrollment and returns one-time recovery codes.
func (s *AuthService) ActivateTotp(ctx context.Context, req *connect.Request[zerxv1.ActivateTotpRequest]) (*connect.Response[zerxv1.ActivateTotpResponse], error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	tt, err := gorm.G[model.UserTOTP](s.db).Where("user_id = ?", claims.UserID).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("请先初始化两步验证"))
	}
	if !totp.Validate(req.Msg.GetCode(), tt.Secret) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("验证码错误"))
	}
	now := time.Now()
	codes := make([]string, 0, 8)
	rows := make([]model.TOTPRecoveryCode, 0, 8)
	for i := 0; i < 8; i++ {
		raw := make([]byte, 6)
		if _, err := rand.Read(raw); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		code := hex.EncodeToString(raw)
		codes = append(codes, code)
		h, err := auth.Hash(code)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		rows = append(rows, model.TOTPRecoveryCode{UserID: claims.UserID, CodeHash: h})
	}
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.UserTOTP{}).Where("user_id = ?", claims.UserID).
			Updates(map[string]any{"enabled": true, "confirmed_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", claims.UserID).Delete(&model.TOTPRecoveryCode{}).Error; err != nil {
			return err
		}
		return tx.CreateInBatches(&rows, 8).Error
	})
	if txErr != nil {
		return nil, connect.NewError(connect.CodeInternal, txErr)
	}

	return connect.NewResponse(&zerxv1.ActivateTotpResponse{RecoveryCodes: codes}), nil
}

// DisableTotp turns off the caller's 2FA after verifying a code.
func (s *AuthService) DisableTotp(ctx context.Context, req *connect.Request[zerxv1.DisableTotpRequest]) (*connect.Response[zerxv1.DisableTotpResponse], error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	tt, err := gorm.G[model.UserTOTP](s.db).Where("user_id = ?", claims.UserID).First(ctx)
	if err != nil {
		return connect.NewResponse(&zerxv1.DisableTotpResponse{}), nil //nolint:nilerr // already disabled
	}
	code := req.Msg.GetCode()
	if !totp.Validate(code, tt.Secret) && !s.consumeRecoveryCode(ctx, claims.UserID, code) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("验证码错误"))
	}
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", claims.UserID).Delete(&model.UserTOTP{}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", claims.UserID).Delete(&model.TOTPRecoveryCode{}).Error
	})
	if txErr != nil {
		return nil, connect.NewError(connect.CodeInternal, txErr)
	}

	return connect.NewResponse(&zerxv1.DisableTotpResponse{}), nil
}
