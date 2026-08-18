// Package ratelimit provides a DB-backed brute-force guard for login attempts so
// failure counts aggregate across instances (preventing the lock threshold from
// being diluted N-fold under multiple replicas). State is reclaimed by TTL.
package ratelimit

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zerx-lab/zkit/internal/model"
)

// LoginGuard tracks failed attempts per key (typically "email|ip") within a
// window anchored at the first failure (length lockFor), requiring a captcha
// after captchaThreshold failures and locking after lockThreshold failures. All
// state lives in the login_attempts table, shared across instances.
type LoginGuard struct {
	db               *gorm.DB
	captchaThreshold int
	lockThreshold    int
	lockFor          time.Duration
}

// New constructs a LoginGuard backed by db.
func New(captchaThreshold, lockThreshold int, lockFor time.Duration, db *gorm.DB) *LoginGuard {
	return &LoginGuard{
		db:               db,
		captchaThreshold: captchaThreshold,
		lockThreshold:    lockThreshold,
		lockFor:          lockFor,
	}
}

// current returns the live (non-expired) attempt row for key, or nil if absent
// or expired.
func (g *LoginGuard) current(key string) *model.LoginAttempt {
	var a model.LoginAttempt
	err := g.db.Where("attempt_key = ? AND expires_at > ?", key, time.Now()).First(&a).Error
	if err != nil {
		return nil
	}

	return &a
}

// Fail records a failed attempt for key, anchoring a new window when none is
// active.
func (g *LoginGuard) Fail(key string) {
	now := time.Now()
	res := g.db.Model(&model.LoginAttempt{}).
		Where("attempt_key = ? AND expires_at > ?", key, now).
		UpdateColumn("fails", gorm.Expr("fails + 1"))
	if res.Error == nil && res.RowsAffected > 0 {
		return
	}

	// No active window (row missing or expired): start a fresh one, overwriting
	// any stale row.
	g.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "attempt_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"fails", "first_at", "expires_at"}),
	}).Create(&model.LoginAttempt{
		AttemptKey: key,
		Fails:      1,
		FirstAt:    now,
		ExpiresAt:  now.Add(g.lockFor),
	})
}

// NeedCaptcha reports whether a captcha is required for key.
func (g *LoginGuard) NeedCaptcha(key string) bool {
	a := g.current(key)
	return a != nil && a.Fails >= g.captchaThreshold
}

// Locked reports whether key is locked and, if so, the remaining lock duration.
func (g *LoginGuard) Locked(key string) (bool, time.Duration) {
	a := g.current(key)
	if a == nil || a.Fails < g.lockThreshold {
		return false, 0
	}

	return true, time.Until(a.ExpiresAt)
}

// Reset clears all recorded attempts for key (called on successful login).
func (g *LoginGuard) Reset(key string) {
	g.db.Where("attempt_key = ?", key).Delete(&model.LoginAttempt{})
}
