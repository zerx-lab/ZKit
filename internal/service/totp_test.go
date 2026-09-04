package service

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	gotptotp "github.com/pquerna/otp/totp"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/model"
)

func testAuthCfg() config.AuthConfig {
	return config.AuthConfig{CaptchaThreshold: 99, LockThreshold: 99, LockFor: time.Hour}
}

// seedTOTP inserts an enabled UserTOTP row for userID and returns its secret.
func seedTOTP(t *testing.T, svc *AuthService, userID uint64) string {
	t.Helper()
	key, err := gotptotp.Generate(gotptotp.GenerateOpts{Issuer: "ZKit", AccountName: "t@x.com"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := svc.db.Create(&model.UserTOTP{UserID: userID, Secret: key.Secret(), Enabled: true}).Error; err != nil {
		t.Fatalf("insert UserTOTP: %v", err)
	}
	return key.Secret()
}

func codeAt(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	code, err := gotptotp.GenerateCode(secret, at)
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	return code
}

func lastUsedStep(t *testing.T, svc *AuthService, userID uint64) int64 {
	t.Helper()
	var tt model.UserTOTP
	if err := svc.db.Where("user_id = ?", userID).First(&tt).Error; err != nil {
		t.Fatalf("read UserTOTP: %v", err)
	}
	return tt.LastUsedStep
}

func TestMatchTOTPStepSkewWindow(t *testing.T) {
	key, err := gotptotp.Generate(gotptotp.GenerateOpts{Issuer: "ZKit", AccountName: "t@x.com"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	secret := key.Secret()
	// Pin `now` to the middle of a step so ±1 boundaries are unambiguous.
	now := time.Unix(1_700_000_015, 0)
	cur := now.Unix() / 30

	for _, delta := range []int64{-1, 0, 1} {
		want := cur + delta
		code := codeAt(t, secret, time.Unix(want*30, 0))
		step, ok := matchTOTPStep(secret, code, now)
		if !ok || step != want {
			t.Errorf("delta %d: got (step=%d, ok=%v), want (step=%d, ok=true)", delta, step, ok, want)
		}
	}
	for _, delta := range []int64{-2, 2} {
		code := codeAt(t, secret, time.Unix((cur+delta)*30, 0))
		if _, ok := matchTOTPStep(secret, code, now); ok {
			t.Errorf("delta %d: code outside skew window accepted", delta)
		}
	}
	if _, ok := matchTOTPStep(secret, "12345", now); ok {
		t.Error("5-digit code accepted")
	}
}

func TestConsumeTOTPRejectsReplay(t *testing.T) {
	db := newTestDB(t)
	svc := newAuthService(t, db, testAuthCfg())
	secret := seedTOTP(t, svc, 1)
	ctx := context.Background()

	now := time.Now()
	code := codeAt(t, secret, now)
	if !svc.consumeTOTP(ctx, 1, secret, code) {
		t.Fatal("first use of a fresh code rejected")
	}
	if got, want := lastUsedStep(t, svc, 1), now.Unix()/30; got != want {
		t.Fatalf("last_used_step = %d, want %d", got, want)
	}
	if svc.consumeTOTP(ctx, 1, secret, code) {
		t.Fatal("replayed code accepted")
	}
}

// A code from the previous step is accepted once (skew), and because the
// matching step — not the current step — is recorded, the current step's code
// remains usable afterwards. The reverse order is rejected: once the current
// step is recorded, the older step's code is stale.
func TestConsumeTOTPRecordsMatchingStep(t *testing.T) {
	db := newTestDB(t)
	svc := newAuthService(t, db, testAuthCfg())
	secret := seedTOTP(t, svc, 1)
	ctx := context.Background()

	// Avoid a step boundary flipping between the two calls.
	now := time.Now()
	if now.Unix()%30 > 25 {
		time.Sleep(time.Duration(30-now.Unix()%30) * time.Second)
		now = time.Now()
	}
	cur := now.Unix() / 30
	prevCode := codeAt(t, secret, time.Unix((cur-1)*30, 0))
	curCode := codeAt(t, secret, time.Unix(cur*30, 0))

	if !svc.consumeTOTP(ctx, 1, secret, prevCode) {
		t.Fatal("previous-step code rejected within skew window")
	}
	if got := lastUsedStep(t, svc, 1); got != cur-1 {
		t.Fatalf("last_used_step = %d, want matched step %d (not current %d)", got, cur-1, cur)
	}
	if svc.consumeTOTP(ctx, 1, secret, prevCode) {
		t.Fatal("previous-step code replayed")
	}
	if !svc.consumeTOTP(ctx, 1, secret, curCode) {
		t.Fatal("current-step code rejected after consuming previous-step code")
	}
	if got := lastUsedStep(t, svc, 1); got != cur {
		t.Fatalf("last_used_step = %d, want %d", got, cur)
	}
	// Going backwards is now impossible: step cur-1 <= recorded cur.
	if svc.consumeTOTP(ctx, 1, secret, prevCode) {
		t.Fatal("older-step code accepted after a newer step was recorded")
	}
}

func TestConsumeTOTPUnknownUser(t *testing.T) {
	db := newTestDB(t)
	svc := newAuthService(t, db, testAuthCfg())
	secret := seedTOTP(t, svc, 1)
	// Valid code but no user_totps row for user 2: conditional UPDATE hits 0 rows.
	if svc.consumeTOTP(context.Background(), 2, secret, codeAt(t, secret, time.Now())) {
		t.Fatal("code accepted for user without a TOTP row")
	}
}

func TestLoginRejectsReplayedTotpCode(t *testing.T) {
	db := newTestDB(t)
	svc := newAuthService(t, db, testAuthCfg())
	seedUser(t, db, "replay@x.com", "password12", "user")
	var u model.User
	if err := db.Where("email = ?", "replay@x.com").First(&u).Error; err != nil {
		t.Fatalf("read user: %v", err)
	}
	secret := seedTOTP(t, svc, u.ID)
	ctx := context.Background()

	code := codeAt(t, secret, time.Now())
	login := func() (*connect.Response[zerxv1.LoginResponse], error) {
		return svc.Login(ctx, connect.NewRequest(&zerxv1.LoginRequest{
			Email: "replay@x.com", Password: "password12", TotpCode: code,
		}))
	}

	res, err := login()
	if err != nil {
		t.Fatalf("first login with fresh code: %v", err)
	}
	if res.Msg.GetAccessToken() == "" {
		t.Fatal("first login issued no access token")
	}

	_, err = login()
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("second login with same code: got code %v (err=%v), want Unauthenticated", connect.CodeOf(err), err)
	}
}
