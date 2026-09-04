package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/database"
	"github.com/zerx-lab/zkit/internal/jobs"
	"github.com/zerx-lab/zkit/internal/model"
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
	cfg.Server.MaxRequestBytes = 1 << 20
	cfg.Server.SecurityHeaders = true
	srv, err := New(cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, jobs.NewRegistry(db), pluginState)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if srv == nil || srv.Handler == nil {
		t.Fatal("New returned nil handler")
	}
	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	// Middleware contract: request id echoed, security headers present on the
	// SPA fallback, HSTS absent over plain HTTP.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-Id", "abc-123")
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz: %d", rec.Code)
	}
	if got := rec.Header().Get("X-Request-Id"); got != "abc-123" {
		t.Fatalf("request id not echoed: %q", got)
	}
	if rec.Header().Get("Content-Security-Policy") == "" || rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("security headers missing: %v", rec.Header())
	}
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS must not be set over plain HTTP")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-Id", "../../evil\n")
	srv.Handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-Id"); got == "" || got == "../../evil\n" {
		t.Fatalf("invalid inbound id must be replaced, got %q", got)
	}
}

func TestErrorSanitizerHidesInternalDetails(t *testing.T) {
	leak := errors.New("pq: relation \"users\" does not exist")
	next := func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, connect.NewError(connect.CodeInternal, leak)
	}
	_, err := NewErrorSanitizerInterceptor()(next)(context.Background(), connect.NewRequest(&struct{}{}))
	if err == nil || connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("code changed: %v", err)
	}
	if strings.Contains(err.Error(), "relation") {
		t.Fatalf("internal detail leaked: %v", err)
	}

	next = func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	_, err = NewErrorSanitizerInterceptor()(next)(context.Background(), connect.NewRequest(&struct{}{}))
	if err == nil || !strings.Contains(err.Error(), "user not found") {
		t.Fatalf("non-internal message must pass through: %v", err)
	}
}

func TestOpLogWriterDropsOnOverflowAndDrains(t *testing.T) {
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.OperationLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	w := newOpLogWriter(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for range opLogQueueSize * 2 {
		w.enqueue(model.OperationLog{Procedure: "/x/Y", Status: "ok"})
	}
	if err := w.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
	n, err := gorm.G[model.OperationLog](db).Count(context.Background(), "id")
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 || n > opLogQueueSize*2 {
		t.Fatalf("persisted %d rows", n)
	}
	if n+w.dropped.Load() != opLogQueueSize*2 {
		t.Fatalf("persisted %d + dropped %d != enqueued %d", n, w.dropped.Load(), opLogQueueSize*2)
	}
	// After Close, enqueue is a no-op (no panic on closed channel).
	w.enqueue(model.OperationLog{Procedure: "/x/Z"})
}
