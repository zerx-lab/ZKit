// Package captcha wraps base64Captcha with a DB-backed, TTL'd store so codes are
// shared across instances: an instance may generate a captcha that another
// verifies. Codes do not survive their TTL and are reclaimed by a cleanup job.
package captcha

import (
	"context"
	"time"

	"github.com/mojocn/base64Captcha"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zerx-lab/zkit/internal/model"
)

// captchaTTL is how long a generated code remains verifiable.
const captchaTTL = 3 * time.Minute

// Manager generates and verifies digit captchas over a shared DB store.
type Manager struct {
	cap   *base64Captcha.Captcha
	store base64Captcha.Store
}

// dbStore implements base64Captcha.Store over the captcha_codes table, making
// codes cross-instance verifiable.
type dbStore struct {
	db *gorm.DB
}

// Set upserts a code answer with a fresh TTL.
func (s *dbStore) Set(id, value string) error {
	return s.db.WithContext(context.Background()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"answer", "expires_at"}),
	}).Create(&model.CaptchaCode{
		ID:        id,
		Answer:    value,
		ExpiresAt: time.Now().Add(captchaTTL),
	}).Error
}

// Get returns the stored answer for id, or "" if missing/expired. When clear is
// set, the row is deleted regardless of return value.
func (s *dbStore) Get(id string, clear bool) string {
	var code model.CaptchaCode
	err := s.db.WithContext(context.Background()).
		Where("id = ? AND expires_at > ?", id, time.Now()).
		First(&code).Error
	if err != nil {
		return ""
	}
	if clear {
		s.db.WithContext(context.Background()).Where("id = ?", id).Delete(&model.CaptchaCode{})
	}

	return code.Answer
}

// Verify reports whether answer matches the stored code (case-sensitive),
// clearing the code when clear is set.
func (s *dbStore) Verify(id, answer string, clear bool) bool {
	got := s.Get(id, clear)
	return got != "" && got == answer
}

// New builds a Manager with a 5-digit driver backed by a DB store.
func New(db *gorm.DB) *Manager {
	store := &dbStore{db: db}
	driver := base64Captcha.NewDriverDigit(80, 240, 5, 0.7, 80)
	cap := base64Captcha.NewCaptcha(driver, store)

	return &Manager{cap: cap, store: store}
}

// Generate returns a captcha id and a base64-encoded PNG data URL. The answer is
// kept server-side in the store.
func (m *Manager) Generate() (id, b64 string, err error) {
	id, b64, _, err = m.cap.Generate()
	return id, b64, err
}

// Verify checks an answer for the given id, clearing it on success or failure
// (one-shot).
func (m *Manager) Verify(id, answer string) bool {
	if id == "" || answer == "" {
		return false
	}

	return m.store.Verify(id, answer, true)
}
