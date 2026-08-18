// Package param provides a runtime cache over system parameters, kept in sync
// with the SysParam table. State is process-local (no cross-instance coherence).
package param

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zerx-lab/zkit/internal/model"
)

// Cache holds system parameter key/value pairs in memory.
type Cache struct {
	mu     sync.RWMutex
	db     *gorm.DB
	values map[string]string
}

// New builds an empty cache bound to db. Call Load to populate it.
func New(db *gorm.DB) *Cache {
	return &Cache{db: db, values: make(map[string]string)}
}

// Load reads all parameters from the database into the cache.
func (c *Cache) Load(ctx context.Context) error {
	params, err := gorm.G[model.SysParam](c.db).Find(ctx)
	if err != nil {
		return fmt.Errorf("load params: %w", err)
	}

	values := make(map[string]string, len(params))
	for i := range params {
		values[params[i].Key] = params[i].Value
	}

	c.mu.Lock()
	c.values = values
	c.mu.Unlock()

	return nil
}

// Get returns the cached value for key.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	v, ok := c.values[key]
	return v, ok
}

// Set upserts a parameter value in the database and refreshes the cache entry.
func (c *Cache) Set(ctx context.Context, key, val string) error {
	if err := c.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&model.SysParam{Key: key, Value: val}).Error; err != nil {
		return fmt.Errorf("upsert param: %w", err)
	}

	c.mu.Lock()
	c.values[key] = val
	c.mu.Unlock()

	return nil
}

// Reload re-reads the whole table into the cache.
func (c *Cache) Reload(ctx context.Context) error {
	return c.Load(ctx)
}

// StartReloader 周期性从库重载缓存,直到 ctx 结束。它是【每实例】goroutine,
// 绝不可注册为 ScheduledJob —— 否则会被分布式锁锁住,只剩一个实例刷新,适得其反。
func (c *Cache) StartReloader(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = c.Reload(ctx) // best-effort;出错保留上次值,下个 tick 重试
		}
	}
}
