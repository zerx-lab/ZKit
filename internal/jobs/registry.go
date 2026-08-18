// Package jobs provides a cron scheduler over DB-registered ScheduledJob rows
// and an in-process registry of named handler functions. The UI can only
// schedule handlers present in the registry.
package jobs

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/model"
)

// HandlerFunc is a job body.
type HandlerFunc func(ctx context.Context) error

// Descriptor pairs a handler with a human-readable description.
type Descriptor struct {
	Handler     HandlerFunc
	Description string
}

// Registry maps handler keys to descriptors.
type Registry map[string]Descriptor

// NewRegistry builds the registry with the built-in handlers bound to db.
func NewRegistry(db *gorm.DB) Registry {
	return Registry{
		"log_cleanup": {
			Description: "清理 30 天前的操作日志与登录日志",
			Handler: func(ctx context.Context) error {
				cutoff := time.Now().AddDate(0, 0, -30)
				if err := db.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&model.OperationLog{}).Error; err != nil {
					return err
				}
				return db.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&model.LoginLog{}).Error
			},
		},
		"session_cleanup": {
			Description: "清理已过期的用户会话",
			Handler: func(ctx context.Context) error {
				return db.WithContext(ctx).Where("expires_at < ?", time.Now()).Delete(&model.UserSession{}).Error
			},
		},
		"auth_state_cleanup": {
			Description: "清理过期的验证码与登录失败记录",
			Handler: func(ctx context.Context) error {
				now := time.Now()
				if err := db.WithContext(ctx).Where("expires_at < ?", now).Delete(&model.CaptchaCode{}).Error; err != nil {
					return err
				}
				return db.WithContext(ctx).Where("expires_at < ?", now).Delete(&model.LoginAttempt{}).Error
			},
		},
	}
}

// ValidCron reports whether expr is a cron expression gocron can schedule
// (5-field standard cron, no seconds).
func ValidCron(expr string) bool {
	_, err := cron.ParseStandard(expr)
	return err == nil
}
