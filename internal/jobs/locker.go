package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/go-co-op/gocron/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zerx-lab/zkit/internal/model"
)

// errLockHeld signals that another instance currently owns the job lock, so this
// fire is skipped (gocron skips the run when Lock returns an error).
var errLockHeld = errors.New("job lock held by another instance")

// dbLocker is a gocron distributed locker backed by the job_locks table. It is
// portable across sqlite/pg/mysql: claiming relies on the RowsAffected of a
// conditional UPDATE (not on driver-specific error translation). Locks are
// released only by TTL expiry — never DELETE/Unlock — which tolerates clock
// skew between instances and never double-runs.
//
// Constraint: ttl must satisfy maxClockSkew (NTP < 1s) < ttl < the smallest
// cron interval (60s for 5-field CronJob). Built-in cleanup jobs run sub-second,
// far below the 30s ttl, so no in-flight job is double-fired. A new job whose
// runtime exceeds the ttl, deployed multi-replica, requires raising the ttl.
type dbLocker struct {
	db    *gorm.DB
	owner string
	ttl   time.Duration
}

func (l *dbLocker) Lock(ctx context.Context, key string) (gocron.Lock, error) {
	now := time.Now()

	// Ensure a row exists (expired epoch) without disturbing an active lease.
	if err := l.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&model.JobLock{LockKey: key, ExpiresAt: time.Unix(0, 0)}).Error; err != nil {
		return nil, err
	}

	res := l.db.WithContext(ctx).Model(&model.JobLock{}).
		Where("lock_key = ? AND expires_at < ?", key, now).
		Updates(map[string]any{"owner": l.owner, "acquired_at": now, "expires_at": now.Add(l.ttl)})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, errLockHeld
	}

	return noopLock{}, nil
}

type noopLock struct{}

// Unlock is intentionally a no-op: the lock is released only by TTL expiry,
// which tolerates clock skew between instances and never double-runs.
func (noopLock) Unlock(context.Context) error { return nil }
