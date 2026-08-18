package service

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/captcha"
	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/mailer"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/param"
	"github.com/zerx-lab/zkit/internal/ratelimit"
	"github.com/zerx-lab/zkit/internal/storage"
)

func newAuthService(t *testing.T, db *gorm.DB, cfg config.AuthConfig) *AuthService {
	t.Helper()
	issuer := auth.NewIssuer(config.JWTConfig{Secret: "test-secret", AccessTTL: 15 * time.Minute, RefreshTTL: time.Hour})
	guard := ratelimit.New(cfg.CaptchaThreshold, cfg.LockThreshold, cfg.LockFor, db)
	policy := auth.NewPolicy(config.PasswordPolicyConfig{MinLength: 8, HistoryCount: 3})
	m := mailer.NewMailer(config.SMTPConfig{}, slog.Default())
	paramCache := param.New(db)
	_ = paramCache.Load(context.Background())
	store, _ := storage.New(config.StorageConfig{Driver: "local", LocalDir: t.TempDir(), LocalBaseURL: "/uploads"})
	mr := media.New(store, config.StorageConfig{LocalBaseURL: "/uploads", SignedURLTTL: time.Hour}, []byte("test-sign-key"))
	return NewAuthService(db, issuer, guard, captcha.New(db), cfg, m, policy, paramCache, mr)
}

func seedUser(t *testing.T, db *gorm.DB, email, password, role string) {
	t.Helper()
	hash, err := auth.Hash(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := model.User{Email: email, Name: "U", PasswordHash: hash, Status: true}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if role != "" {
		if err := db.Create(&model.UserRole{UserID: u.ID, RoleCode: role}).Error; err != nil {
			t.Fatalf("create user_role: %v", err)
		}
	}
}

func TestLoginCaptchaThreshold(t *testing.T) {
	db := newTestDB(t)
	cfg := config.AuthConfig{CaptchaThreshold: 2, LockThreshold: 5, LockFor: time.Hour}
	svc := newAuthService(t, db, cfg)
	seedUser(t, db, "a@b.com", "password1", "user")
	ctx := context.Background()

	// Two bad attempts to cross the captcha threshold.
	for range 2 {
		_, _ = svc.Login(ctx, connect.NewRequest(&zerxv1.LoginRequest{Email: "a@b.com", Password: "wrongpass1"}))
	}

	// Now a correct password but missing captcha must be rejected with InvalidArgument.
	_, err := svc.Login(ctx, connect.NewRequest(&zerxv1.LoginRequest{Email: "a@b.com", Password: "password1"}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("login without captcha after threshold = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestLoginSuccessCreatesSessionAndLog(t *testing.T) {
	db := newTestDB(t)
	cfg := config.AuthConfig{CaptchaThreshold: 99, LockThreshold: 99, LockFor: time.Hour}
	svc := newAuthService(t, db, cfg)
	seedUser(t, db, "a@b.com", "password1", "user")
	ctx := context.Background()

	res, err := svc.Login(ctx, connect.NewRequest(&zerxv1.LoginRequest{Email: "a@b.com", Password: "password1"}))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.Msg.GetSessionId() == "" || res.Msg.GetAccessToken() == "" {
		t.Fatal("login response missing session id or token")
	}

	var sessions int64
	db.Model(&model.UserSession{}).Count(&sessions)
	if sessions != 1 {
		t.Errorf("sessions = %d, want 1", sessions)
	}
}

func TestLoginSingleSessionEvictsOthers(t *testing.T) {
	db := newTestDB(t)
	cfg := config.AuthConfig{CaptchaThreshold: 99, LockThreshold: 99, LockFor: time.Hour, SingleSession: true}
	svc := newAuthService(t, db, cfg)
	seedUser(t, db, "a@b.com", "password1", "user")
	ctx := context.Background()

	first, err := svc.Login(ctx, connect.NewRequest(&zerxv1.LoginRequest{Email: "a@b.com", Password: "password1"}))
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	second, err := svc.Login(ctx, connect.NewRequest(&zerxv1.LoginRequest{Email: "a@b.com", Password: "password1"}))
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	var sessions int64
	db.Model(&model.UserSession{}).Count(&sessions)
	// Single session: exactly one survives, and it is the most recent one (never 0).
	if sessions != 1 {
		t.Fatalf("single-session sessions = %d, want 1", sessions)
	}
	if first.Msg.GetSessionId() == second.Msg.GetSessionId() {
		t.Fatal("second login should mint a new session id")
	}

	var remaining model.UserSession
	if err := db.First(&remaining).Error; err != nil {
		t.Fatalf("read remaining session: %v", err)
	}
	if remaining.ID != second.Msg.GetSessionId() {
		t.Errorf("surviving session = %q, want the newest %q", remaining.ID, second.Msg.GetSessionId())
	}
}
