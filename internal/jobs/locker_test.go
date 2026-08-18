package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zerx-lab/zkit/internal/model"
)

// TestDBLockerMutualExclusion proves that of two instances racing for the same
// lock key, exactly one acquires it; the other is rejected; expiry lets another
// owner take over; and an active lease blocks re-acquisition.
func TestDBLockerMutualExclusion(t *testing.T) {
	db := newJobDB(t)
	a := &dbLocker{db: db, owner: "owner-a", ttl: 30 * time.Second}
	b := &dbLocker{db: db, owner: "owner-b", ttl: 30 * time.Second}
	ctx := context.Background()
	const key = "cleanup"

	la, errA := a.Lock(ctx, key)
	lb, errB := b.Lock(ctx, key)

	got := 0
	if errA == nil && la != nil {
		got++
	}
	if errB == nil && lb != nil {
		got++
	}
	if got != 1 {
		t.Fatalf("exactly one Lock must succeed, got %d (errA=%v errB=%v)", got, errA, errB)
	}
	if !errors.Is(errA, errLockHeld) && !errors.Is(errB, errLockHeld) {
		t.Fatalf("the loser must return errLockHeld (errA=%v errB=%v)", errA, errB)
	}

	// Active lease blocks re-acquisition by the other owner.
	if _, err := b.Lock(ctx, key); !errors.Is(err, errLockHeld) {
		t.Fatalf("active lease must block re-acquisition, got %v", err)
	}

	// Force the lease into the past; the other owner can take over.
	if err := db.Model(&model.JobLock{}).Where("lock_key = ?", key).
		Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	if l, err := b.Lock(ctx, key); err != nil || l == nil {
		t.Fatalf("expired lease must be claimable, got l=%v err=%v", l, err)
	}
}
