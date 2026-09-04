package param

import (
	"context"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/model"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.SysParam{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seed(t *testing.T, db *gorm.DB, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		if err := db.Create(&model.SysParam{Key: k, Name: k, Value: v}).Error; err != nil {
			t.Fatalf("seed %s: %v", k, err)
		}
	}
}

func TestLoadThenGet(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, map[string]string{"site.name": "ZKit", "site.limit": "10"})
	ctx := context.Background()

	c := New(db)
	if _, ok := c.Get("site.name"); ok {
		t.Fatal("Get should miss before Load")
	}

	if err := c.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v, ok := c.Get("site.name"); !ok || v != "ZKit" {
		t.Fatalf("Get(site.name) = %q,%v; want ZKit,true", v, ok)
	}
	if v, ok := c.Get("site.limit"); !ok || v != "10" {
		t.Fatalf("Get(site.limit) = %q,%v; want 10,true", v, ok)
	}
}

func TestGetMissingKey(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, map[string]string{"present": "yes"})
	c := New(db)
	if err := c.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	v, ok := c.Get("absent")
	if ok {
		t.Fatalf("Get(absent) reported hit with %q", v)
	}
	if v != "" {
		t.Fatalf("Get(absent) value = %q, want empty", v)
	}
}

func TestSetUpdatesCacheAndPersists(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, map[string]string{"k": "old"})
	ctx := context.Background()

	c := New(db)
	if err := c.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Upsert on existing key.
	if err := c.Set(ctx, "k", "new"); err != nil {
		t.Fatalf("Set existing: %v", err)
	}
	if v, _ := c.Get("k"); v != "new" {
		t.Fatalf("Get(k) after Set = %q, want new", v)
	}

	// Insert of new key.
	if err := c.Set(ctx, "fresh", "1"); err != nil {
		t.Fatalf("Set new: %v", err)
	}
	if v, ok := c.Get("fresh"); !ok || v != "1" {
		t.Fatalf("Get(fresh) = %q,%v; want 1,true", v, ok)
	}

	// Must not duplicate rows for the same key.
	var n int64
	if err := db.Model(&model.SysParam{}).Where("key = ?", "k").Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows for key k = %d, want 1 (upsert)", n)
	}

	// A second cache over the same DB observes the persisted values.
	other := New(db)
	if err := other.Load(ctx); err != nil {
		t.Fatalf("Load other: %v", err)
	}
	if v, _ := other.Get("k"); v != "new" {
		t.Fatalf("persisted k = %q, want new", v)
	}
	if v, _ := other.Get("fresh"); v != "1" {
		t.Fatalf("persisted fresh = %q, want 1", v)
	}
}

func TestReloadPicksUpExternalChanges(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, map[string]string{"k": "v1", "gone": "x"})
	ctx := context.Background()

	c := New(db)
	if err := c.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Out-of-band change (e.g. another instance).
	if err := db.Model(&model.SysParam{}).Where("key = ?", "k").Update("value", "v2").Error; err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := db.Where("key = ?", "gone").Delete(&model.SysParam{}).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := db.Create(&model.SysParam{Key: "added", Name: "added", Value: "a"}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// Stale until Reload.
	if v, _ := c.Get("k"); v != "v1" {
		t.Fatalf("Get(k) before Reload = %q, want stale v1", v)
	}

	if err := c.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if v, _ := c.Get("k"); v != "v2" {
		t.Fatalf("Get(k) after Reload = %q, want v2", v)
	}
	if _, ok := c.Get("gone"); ok {
		t.Fatal("soft-deleted key should disappear after Reload")
	}
	if v, ok := c.Get("added"); !ok || v != "a" {
		t.Fatalf("Get(added) = %q,%v; want a,true", v, ok)
	}
}

func TestLoadReplacesSnapshotAtomically(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, map[string]string{"k": "v"})
	ctx := context.Background()

	c := New(db)
	if err := c.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Cache-only value (not in DB) must be dropped by Load since Load is a full snapshot.
	c.mu.Lock()
	c.values["ephemeral"] = "x"
	c.mu.Unlock()

	if err := c.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := c.Get("ephemeral"); ok {
		t.Fatal("Load must replace the whole snapshot, not merge")
	}
}

func TestConcurrentGetSetReload(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, map[string]string{"k": "0"})
	ctx := context.Background()

	c := New(db)
	if err := c.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}

	var wg sync.WaitGroup
	const readers = 8
	const iters = 200

	for range readers {
		wg.Go(func() {
			for range iters {
				if _, ok := c.Get("k"); !ok {
					t.Error("k vanished during concurrent access")
					return
				}
			}
		})
	}
	wg.Go(func() {
		for i := range 20 {
			if err := c.Set(ctx, "k", string(rune('a'+i))); err != nil {
				t.Errorf("Set: %v", err)
				return
			}
		}
	})
	wg.Go(func() {
		for range 20 {
			if err := c.Reload(ctx); err != nil {
				t.Errorf("Reload: %v", err)
				return
			}
		}
	})
	wg.Wait()
}

func TestSetRevivesSoftDeletedKey(t *testing.T) {
	db := newTestDB(t)
	seed(t, db, map[string]string{"site.name": "old"})
	ctx := context.Background()
	if err := db.Where("key = ?", "site.name").Delete(&model.SysParam{}).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	c := New(db)
	if err := c.Set(ctx, "site.name", "new"); err != nil {
		t.Fatalf("Set on soft-deleted key: %v", err)
	}
	if err := c.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if v, ok := c.Get("site.name"); !ok || v != "new" {
		t.Fatalf("revived key lost after Reload: %q %v", v, ok)
	}
	var n int64
	if err := db.Model(&model.SysParam{}).Where("key = ?", "site.name").Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("expected exactly one live row, got %d (%v)", n, err)
	}
}
