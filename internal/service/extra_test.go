package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"connectrpc.com/connect"
	gotptotp "github.com/pquerna/otp/totp"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/model"
)

// ---------------------------------------------------------------------------
// ChangePassword
// ---------------------------------------------------------------------------

func TestChangePasswordWrongOldPassword(t *testing.T) {
	db := newTestDB(t)
	cfg := config.AuthConfig{CaptchaThreshold: 99, LockThreshold: 99, LockFor: time.Hour}
	svc := newAuthService(t, db, cfg)
	seedUser(t, db, "cp@x.com", "correct-pw8", "user")
	ctx := context.Background()

	// Fetch the seeded user to get its ID.
	var u model.User
	db.Where("email = ?", "cp@x.com").First(&u)

	authedCtx := auth.WithClaims(ctx, &auth.Claims{UserID: u.ID, Roles: []string{"user"}})

	_, err := svc.ChangePassword(authedCtx, connect.NewRequest(&zerxv1.ChangePasswordRequest{
		OldPassword: "wrong-old-8",
		NewPassword: "new-password9",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("wrong old password: got code %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestChangePasswordSuccessClearsSessions(t *testing.T) {
	db := newTestDB(t)
	cfg := config.AuthConfig{CaptchaThreshold: 99, LockThreshold: 99, LockFor: time.Hour}
	svc := newAuthService(t, db, cfg)
	seedUser(t, db, "cp2@x.com", "old-pass-8", "user")
	ctx := context.Background()

	// Login to create a session.
	res, err := svc.Login(ctx, connect.NewRequest(&zerxv1.LoginRequest{
		Email:    "cp2@x.com",
		Password: "old-pass-8",
	}))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	_ = res

	var u model.User
	db.Where("email = ?", "cp2@x.com").First(&u)

	authedCtx := auth.WithClaims(ctx, &auth.Claims{UserID: u.ID, Roles: []string{"user"}})

	_, err = svc.ChangePassword(authedCtx, connect.NewRequest(&zerxv1.ChangePasswordRequest{
		OldPassword: "old-pass-8",
		NewPassword: "new-pass-99",
	}))
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	var sessions int64
	db.Model(&model.UserSession{}).Where("user_id = ?", u.ID).Count(&sessions)
	if sessions != 0 {
		t.Errorf("after ChangePassword: %d sessions remain, want 0", sessions)
	}
}

// ---------------------------------------------------------------------------
// ConfirmPasswordReset
// ---------------------------------------------------------------------------

func TestConfirmPasswordResetExpiredToken(t *testing.T) {
	db := newTestDB(t)
	cfg := config.AuthConfig{CaptchaThreshold: 99, LockThreshold: 99, LockFor: time.Hour}
	svc := newAuthService(t, db, cfg)
	ctx := context.Background()

	// Manufacture an expired token directly in the DB.
	token := "test-reset-token-expired"
	sum := sha256.Sum256([]byte(token))
	hashHex := hex.EncodeToString(sum[:])
	expired := model.PasswordResetToken{
		TokenHash: hashHex,
		UserID:    1,
		ExpiresAt: time.Now().Add(-time.Hour), // already expired
	}
	db.Create(&expired)

	_, err := svc.ConfirmPasswordReset(ctx, connect.NewRequest(&zerxv1.ConfirmPasswordResetRequest{
		Token:       token,
		NewPassword: "newpass12",
	}))
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expired token: got code %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestConfirmPasswordResetUsedToken(t *testing.T) {
	db := newTestDB(t)
	cfg := config.AuthConfig{CaptchaThreshold: 99, LockThreshold: 99, LockFor: time.Hour}
	svc := newAuthService(t, db, cfg)
	ctx := context.Background()

	token := "test-reset-token-used"
	sum := sha256.Sum256([]byte(token))
	hashHex := hex.EncodeToString(sum[:])
	usedAt := time.Now().Add(-time.Minute)
	used := model.PasswordResetToken{
		TokenHash: hashHex,
		UserID:    1,
		ExpiresAt: time.Now().Add(time.Hour), // still valid time-wise
		UsedAt:    &usedAt,                   // but already consumed
	}
	db.Create(&used)

	_, err := svc.ConfirmPasswordReset(ctx, connect.NewRequest(&zerxv1.ConfirmPasswordResetRequest{
		Token:       token,
		NewPassword: "newpass12",
	}))
	if err == nil {
		t.Fatal("expected error for used token, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("used token: got code %v, want InvalidArgument", connect.CodeOf(err))
	}
}

// ---------------------------------------------------------------------------
// TOTP: SetupTotp → ActivateTotp → Login requires totp_code
// ---------------------------------------------------------------------------

func TestActivateTotpAndLoginRequiresTotp(t *testing.T) {
	db := newTestDB(t)
	cfg := config.AuthConfig{CaptchaThreshold: 99, LockThreshold: 99, LockFor: time.Hour}
	svc := newAuthService(t, db, cfg)
	seedUser(t, db, "totp@x.com", "password12", "user")
	ctx := context.Background()

	// Login to get claims.
	loginRes, err := svc.Login(ctx, connect.NewRequest(&zerxv1.LoginRequest{
		Email:    "totp@x.com",
		Password: "password12",
	}))
	if err != nil {
		t.Fatalf("initial login: %v", err)
	}
	var u model.User
	db.Where("email = ?", "totp@x.com").First(&u)
	authedCtx := auth.WithClaims(ctx, &auth.Claims{UserID: u.ID, Roles: []string{"user"}})
	_ = loginRes

	// Setup TOTP to get secret.
	setupRes, err := svc.SetupTotp(authedCtx, connect.NewRequest(&zerxv1.SetupTotpRequest{}))
	if err != nil {
		t.Fatalf("SetupTotp: %v", err)
	}
	secret := setupRes.Msg.GetSecret()
	if secret == "" {
		t.Fatal("SetupTotp returned empty secret")
	}

	// Generate a valid code and activate.
	code, err := gotptotp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	activateRes, err := svc.ActivateTotp(authedCtx, connect.NewRequest(&zerxv1.ActivateTotpRequest{Code: code}))
	if err != nil {
		t.Fatalf("ActivateTotp: %v", err)
	}

	// Verify UserTOTP.Enabled in DB.
	var ut model.UserTOTP
	if err := db.Where("user_id = ?", u.ID).First(&ut).Error; err != nil {
		t.Fatalf("read UserTOTP: %v", err)
	}
	if !ut.Enabled {
		t.Error("UserTOTP.Enabled = false after ActivateTotp, want true")
	}

	// Verify recovery codes returned.
	if len(activateRes.Msg.GetRecoveryCodes()) == 0 {
		t.Error("ActivateTotp returned no recovery codes")
	}

	// After enabling TOTP, Login without totp_code must return TotpRequired=true.
	loginRes2, err := svc.Login(ctx, connect.NewRequest(&zerxv1.LoginRequest{
		Email:    "totp@x.com",
		Password: "password12",
	}))
	if err != nil {
		t.Fatalf("Login after TOTP enabled: %v", err)
	}
	if !loginRes2.Msg.GetTotpRequired() {
		t.Error("Login without totp_code after 2FA enabled: TotpRequired = false, want true")
	}
	if loginRes2.Msg.GetAccessToken() != "" {
		t.Error("Login without totp_code must not issue an access token")
	}
}

// ---------------------------------------------------------------------------
// DeleteRole: role in use → CodeFailedPrecondition
// ---------------------------------------------------------------------------

func TestDeleteRoleInUseViaUserRoles(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)
	svc := NewRoleService(db, e)
	ctx := context.Background()

	// Create a custom role.
	created, err := svc.CreateRole(ctx, connect.NewRequest(&zerxv1.CreateRoleRequest{
		Code: "contractor", Name: "Contractor",
	}))
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	// Assign it to a user via user_roles.
	u := model.User{Email: "contractor@x.com", Name: "C", PasswordHash: "h", Status: true}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := db.Create(&model.UserRole{UserID: u.ID, RoleCode: "contractor"}).Error; err != nil {
		t.Fatalf("insert user_role: %v", err)
	}

	_, err = svc.DeleteRole(ctx, connect.NewRequest(&zerxv1.DeleteRoleRequest{Id: created.Msg.GetId()}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("delete in-use role: got code %v, want FailedPrecondition", connect.CodeOf(err))
	}
}
