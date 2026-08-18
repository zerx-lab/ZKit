package server

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/database"
	"github.com/zerx-lab/zkit/internal/plugin"
	"github.com/zerx-lab/zkit/internal/plugins"
)

func TestAssertServicesRegisteredDetectsMissing(t *testing.T) {
	err := assertServicesRegistered([]string{"/zerx.v1.UserService/"})
	if err == nil {
		t.Fatal("expected error for unregistered services, got nil")
	}
}

func TestNewRegistersAllServices(t *testing.T) {
	// Register compiled-in plugins so their services are mounted; otherwise
	// assertServicesRegistered (correctly) fails on the unmounted plugin service.
	plugins.Register()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(db, plugin.CollectMigrations()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := &config.Config{}
	cfg.JWT.Secret = "test"
	cfg.Storage = config.StorageConfig{Driver: "local", LocalDir: t.TempDir(), LocalBaseURL: "/uploads"}
	cfg.Auth = config.AuthConfig{CaptchaThreshold: 2, LockThreshold: 5, LockFor: time.Minute}
	cfg.RateLimit = config.RateLimitConfig{Enabled: false, RPS: 20, Burst: 40, TTL: 10 * time.Minute}

	pluginState, err := plugin.NewState(db)
	if err != nil {
		t.Fatalf("plugin state: %v", err)
	}
	handler, err := New(cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, pluginState)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if handler == nil {
		t.Fatal("New returned nil handler")
	}
}
