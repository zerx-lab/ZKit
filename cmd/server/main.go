// Command server is the ZKit all-in-one binary: connectRPC API plus the
// embedded SPA, served over HTTP/1.1 and h2c.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/database"
	"github.com/zerx-lab/zkit/internal/jobs"
	"github.com/zerx-lab/zkit/internal/plugin"
	"github.com/zerx-lab/zkit/internal/plugins"
	"github.com/zerx-lab/zkit/internal/server"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg.Env)

	// Register compiled-in plugins (pure in-memory) and validate them before any
	// DB work. ValidateAll enforces naming/namespacing and the procedure-ownership
	// security boundary; a violation aborts startup.
	plugins.Register()
	reservedNames, reservedPaths := database.ReservedMenuIdentifiers()
	if err := plugin.ValidateAll(plugin.Reserved{MenuNames: reservedNames, MenuPaths: reservedPaths}); err != nil {
		return fmt.Errorf("validate plugins: %w", err)
	}

	db, err := database.Open(cfg.DB, cfg.Env)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if sqlDB, derr := db.DB(); derr == nil {
			_ = sqlDB.Close()
		}
	}()

	if err := database.Migrate(db, plugin.CollectMigrations()); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	if err := database.Seed(db, collectPluginMenus()); err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	// Runtime plugin enable/disable state (loaded from plugin_states); gates
	// procedures (Casbin interceptor), menus (GetUserMenus), and jobs (scheduler).
	pluginState, err := plugin.NewState(db)
	if err != nil {
		return fmt.Errorf("load plugin state: %w", err)
	}
	// On multi-replica drivers, periodically reload so an enable/disable on one
	// replica propagates to the others (single-node stays correct via the
	// in-process SetEnabled update). Mirrors param.Cache.StartReloader.
	if cfg.DB.Driver == "postgres" || cfg.DB.Driver == "mysql" {
		go pluginState.StartReloader(context.Background(), 30*time.Second)
	}

	registry := jobs.NewRegistry(db)
	// Merge plugin job handlers into the scheduler registry: without this the
	// scheduler would log "unknown handler" and silently skip plugin jobs.
	for _, p := range plugin.All() {
		for k, jh := range p.JobHandlers() {
			registry[k] = jobs.Descriptor{Handler: jh.Run, Description: jh.Description}
		}
	}
	scheduler, err := jobs.New(db, registry, logger)
	if err != nil {
		return fmt.Errorf("build scheduler: %w", err)
	}
	scheduler.SetHandlerEnabled(pluginState.IsJobHandlerEnabled)

	handler, err := server.New(cfg, db, logger, scheduler, pluginState)
	if err != nil {
		return fmt.Errorf("build server: %w", err)
	}

	if err := scheduler.Start(); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true) // h2c, for grpc tooling; SPA uses HTTP/1.1

	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "version", version, "addr", cfg.Server.Addr, "env", cfg.Env)
		if serveErr := srv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case serveErr := <-errCh:
		return fmt.Errorf("serve: %w", serveErr)
	}

	_ = scheduler.Shutdown()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	logger.Info("server stopped")

	return nil
}

// collectPluginMenus flattens every registered plugin's SeedMenus into the
// database package's MenuSeed type (the conversion lives here so neither the
// database nor plugin package depends on the other).
func collectPluginMenus() []database.MenuSeed {
	var out []database.MenuSeed
	for _, p := range plugin.All() {
		out = append(out, toMenuSeeds(p.SeedMenus())...)
	}
	return out
}

func toMenuSeeds(in []plugin.MenuNode) []database.MenuSeed {
	out := make([]database.MenuSeed, 0, len(in))
	for i := range in {
		n := in[i]
		buttons := make([]database.MenuButtonSeed, 0, len(n.Buttons))
		for j := range n.Buttons {
			buttons = append(buttons, database.MenuButtonSeed{Code: n.Buttons[j].Code, Name: n.Buttons[j].Name})
		}
		out = append(out, database.MenuSeed{
			Name:        n.Name,
			Path:        n.Path,
			Component:   n.Component,
			Title:       n.Title,
			Icon:        n.Icon,
			Sort:        n.Sort,
			Hidden:      n.Hidden,
			UserVisible: n.UserVisible,
			Buttons:     buttons,
			Children:    toMenuSeeds(n.Children),
		})
	}
	return out
}

func newLogger(env string) *slog.Logger {
	if env == "dev" {
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
