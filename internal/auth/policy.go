package auth

import (
	"context"
	"errors"
	"fmt"
	"unicode"

	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/model"
)

// Policy enforces password strength and reuse rules from configuration.
type Policy struct {
	cfg config.PasswordPolicyConfig
}

// NewPolicy builds a password Policy from configuration.
func NewPolicy(cfg config.PasswordPolicyConfig) *Policy {
	return &Policy{cfg: cfg}
}

// Validate reports whether a plaintext password meets the configured policy.
func (p *Policy) Validate(pw string) error {
	if len(pw) < p.cfg.MinLength {
		return fmt.Errorf("密码长度至少 %d 位", p.cfg.MinLength)
	}
	var hasUpper, hasLower, hasDigit, hasSymbol bool
	for _, r := range pw {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSymbol = true
		}
	}
	if p.cfg.RequireUpper && !hasUpper {
		return errors.New("密码必须包含大写字母")
	}
	if p.cfg.RequireLower && !hasLower {
		return errors.New("密码必须包含小写字母")
	}
	if p.cfg.RequireDigit && !hasDigit {
		return errors.New("密码必须包含数字")
	}
	if p.cfg.RequireSymbol && !hasSymbol {
		return errors.New("密码必须包含特殊字符")
	}

	return nil
}

// CheckHistory rejects a new password matching any of the user's most recent
// HistoryCount stored hashes.
func (p *Policy) CheckHistory(ctx context.Context, db *gorm.DB, userID uint64, newPlain string) error {
	if p.cfg.HistoryCount <= 0 {
		return nil
	}
	var rows []model.PasswordHistory
	if err := db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(p.cfg.HistoryCount).
		Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		if Verify(rows[i].PasswordHash, newPlain) == nil {
			return fmt.Errorf("不能与最近 %d 次使用的密码相同", p.cfg.HistoryCount)
		}
	}

	return nil
}

// RecordHistory appends a password hash and trims entries beyond HistoryCount.
func (p *Policy) RecordHistory(ctx context.Context, db *gorm.DB, userID uint64, hash string) error {
	if p.cfg.HistoryCount <= 0 {
		return nil
	}
	if err := db.WithContext(ctx).Create(&model.PasswordHistory{UserID: userID, PasswordHash: hash}).Error; err != nil {
		return err
	}
	// Trim: keep only the most recent HistoryCount rows.
	var keep []model.PasswordHistory
	if err := db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(p.cfg.HistoryCount).
		Find(&keep).Error; err != nil {
		return err
	}
	if len(keep) < p.cfg.HistoryCount {
		return nil
	}
	oldest := keep[len(keep)-1].CreatedAt
	return db.WithContext(ctx).
		Where("user_id = ? AND created_at < ?", userID, oldest).
		Delete(&model.PasswordHistory{}).Error
}
