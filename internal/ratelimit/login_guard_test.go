package ratelimit

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/database"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(db, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestLoginGuardThresholds(t *testing.T) {
	g := New(2, 4, time.Hour, newTestDB(t))
	const key = "a@b.com|1.2.3.4"

	if g.NeedCaptcha(key) {
		t.Fatal("fresh key should not need captcha")
	}

	g.Fail(key)
	if g.NeedCaptcha(key) {
		t.Fatal("1 fail < captchaThreshold; should not need captcha")
	}
	g.Fail(key)
	if !g.NeedCaptcha(key) {
		t.Fatal("2 fails >= captchaThreshold; should need captcha")
	}
	if locked, _ := g.Locked(key); locked {
		t.Fatal("2 fails < lockThreshold; should not be locked")
	}

	g.Fail(key)
	g.Fail(key)
	if locked, _ := g.Locked(key); !locked {
		t.Fatal("4 fails >= lockThreshold; should be locked")
	}

	g.Reset(key)
	if g.NeedCaptcha(key) {
		t.Fatal("after reset, should not need captcha")
	}
	if locked, _ := g.Locked(key); locked {
		t.Fatal("after reset, should not be locked")
	}
}

func TestLoginGuardWindowReset(t *testing.T) {
	g := New(2, 4, time.Millisecond, newTestDB(t))
	const key = "x"

	g.Fail(key)
	g.Fail(key)
	if !g.NeedCaptcha(key) {
		t.Fatal("should need captcha before window elapses")
	}

	time.Sleep(2 * time.Millisecond)
	// A new fail after the window resets the counter.
	g.Fail(key)
	if g.NeedCaptcha(key) {
		t.Fatal("window elapsed; counter should have reset to 1 fail")
	}
}

// TestLoginGuardCrossInstance proves failure counts aggregate across instances
// sharing the same DB.
func TestLoginGuardCrossInstance(t *testing.T) {
	db := newTestDB(t)
	g1 := New(2, 5, 15*time.Minute, db)
	g2 := New(2, 5, 15*time.Minute, db)
	const key = "c@d.com|9.9.9.9"

	g1.Fail(key)
	g1.Fail(key)
	if !g2.NeedCaptcha(key) {
		t.Fatal("instance B must see failure count from instance A")
	}
}
