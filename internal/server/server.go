// Package server wires the connectRPC handlers, interceptor chain, embedded SPA,
// upload endpoint, and health check into a single HTTP handler.
package server

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/apispec"
	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/captcha"
	"github.com/zerx-lab/zkit/internal/casbin"
	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/jobs"
	"github.com/zerx-lab/zkit/internal/mailer"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/param"
	"github.com/zerx-lab/zkit/internal/plugin"
	"github.com/zerx-lab/zkit/internal/ratelimit"
	"github.com/zerx-lab/zkit/internal/service"
	"github.com/zerx-lab/zkit/internal/storage"
	"github.com/zerx-lab/zkit/internal/web"
)

// New builds the root HTTP handler: connectRPC services under /api, a multipart
// upload endpoint at /api/upload, the embedded SPA at /, and /healthz.
func New(cfg *config.Config, db *gorm.DB, logger *slog.Logger, scheduler *jobs.Scheduler, pluginState *plugin.State) (http.Handler, error) {
	issuer := auth.NewIssuer(cfg.JWT)

	enforcer, err := casbin.New(db)
	if err != nil {
		return nil, err
	}

	guard := ratelimit.New(cfg.Auth.CaptchaThreshold, cfg.Auth.LockThreshold, cfg.Auth.LockFor, db)
	cap := captcha.New(db)
	limiter := ratelimit.NewLimiter(cfg.RateLimit.RPS, cfg.RateLimit.Burst, cfg.RateLimit.TTL)
	policy := auth.NewPolicy(cfg.Password)
	mail := mailer.NewMailer(cfg.SMTP, logger)
	registry := jobs.NewRegistry(db)
	// Merge plugin job handlers so the JobService UI can list them as schedulable.
	// The scheduler that actually executes jobs receives the same merge in main.go.
	for _, p := range plugin.All() {
		for k, jh := range p.JobHandlers() {
			registry[k] = jobs.Descriptor{Handler: jh.Run, Description: jh.Description}
		}
	}

	store, err := storage.New(cfg.Storage)
	if err != nil {
		return nil, err
	}

	var signKey []byte
	if cfg.Storage.Driver == "local" {
		sum := sha256.Sum256([]byte(cfg.JWT.Secret + "/media-url-v1"))
		signKey = sum[:]
	}
	mediaResolver := media.New(store, cfg.Storage, signKey)

	paramCache := param.New(db)
	if err := paramCache.Load(context.Background()); err != nil {
		return nil, err
	}
	// Reload periodically only on multi-replica drivers; single-node sqlite stays query-free.
	if cfg.DB.Driver == "postgres" || cfg.DB.Driver == "mysql" {
		go paramCache.StartReloader(context.Background(), 30*time.Second)
	}

	// public: callable without authentication.
	public := map[string]bool{
		zerxv1connect.AuthServiceLoginProcedure:                   true,
		zerxv1connect.AuthServiceRegisterProcedure:                true,
		zerxv1connect.AuthServiceRefreshProcedure:                 true,
		zerxv1connect.AuthServiceGetCaptchaProcedure:              true,
		zerxv1connect.AuthServiceRequestPasswordResetProcedure:    true,
		zerxv1connect.AuthServiceConfirmPasswordResetProcedure:    true,
		zerxv1connect.SiteSettingsServiceGetSiteSettingsProcedure: true,
		zerxv1connect.PluginServiceListPublicPagesProcedure:       true,
	}

	// selfServe: any authenticated caller is allowed (no Casbin check).
	selfServe := map[string]bool{
		zerxv1connect.AuthServiceMeProcedure:             true,
		zerxv1connect.AuthServiceLogoutProcedure:         true,
		zerxv1connect.AuthServiceListSessionsProcedure:   true,
		zerxv1connect.AuthServiceRevokeSessionProcedure:  true,
		zerxv1connect.MenuServiceGetUserMenusProcedure:   true,
		zerxv1connect.MenuServiceGetUserButtonsProcedure: true,
		zerxv1connect.DictServiceGetDictByTypeProcedure:  true,
		zerxv1connect.AuthServiceChangePasswordProcedure: true,
		zerxv1connect.AuthServiceUpdateProfileProcedure:  true,
		zerxv1connect.AuthServiceSetupTotpProcedure:      true,
		zerxv1connect.AuthServiceActivateTotpProcedure:   true,
		zerxv1connect.AuthServiceDisableTotpProcedure:    true,
	}

	// Merge plugin-declared public/self-serve procedures. ValidateAll (run at
	// startup before any DB work) has already rejected (a) any procedure whose
	// service the plugin does not declare and (b) any declared service not named
	// with the plugin's PascalCase prefix — so a plugin cannot claim a core (or
	// another plugin's) service and thus cannot downgrade a core procedure to
	// public/self-serve here.
	for _, p := range plugin.All() {
		for _, proc := range p.PublicProcedures() {
			public[proc] = true
		}
		for _, proc := range p.SelfServeProcedures() {
			selfServe[proc] = true
		}
	}

	// Interceptor chain (outermost first): logging -> auth -> operation log
	// (also recovers panics) -> casbin -> validate. WithRecover is intentionally
	// omitted; the operation-log interceptor records handler panics with stack.
	chain := []connect.Interceptor{
		NewLoggingInterceptor(logger),
	}
	if cfg.RateLimit.Enabled {
		chain = append(chain, NewRateLimitInterceptor(limiter))
	}
	chain = append(chain,
		auth.NewAuthInterceptor(issuer, public),
		NewOperationLogInterceptor(db),
		auth.NewCasbinInterceptor(enforcer, public, selfServe, pluginState.IsProcedureEnabled),
		validate.NewInterceptor(),
	)
	opts := connect.WithInterceptors(chain...)

	api := http.NewServeMux()
	var registered []string
	reg := func(path string, h http.Handler) {
		registered = append(registered, path)
		api.Handle(path, h)
	}
	reg(zerxv1connect.NewAuthServiceHandler(service.NewAuthService(db, issuer, guard, cap, cfg.Auth, mail, policy, paramCache, mediaResolver), opts))
	reg(zerxv1connect.NewUserServiceHandler(service.NewUserService(db, policy, mediaResolver), opts))
	reg(zerxv1connect.NewRoleServiceHandler(service.NewRoleService(db, enforcer), opts))
	reg(zerxv1connect.NewMenuServiceHandler(service.NewMenuService(db, pluginState), opts))
	reg(zerxv1connect.NewApiServiceHandler(service.NewApiService(db, enforcer), opts))
	reg(zerxv1connect.NewDictServiceHandler(service.NewDictService(db), opts))
	reg(zerxv1connect.NewSysParamServiceHandler(service.NewSysParamService(db, paramCache), opts))
	reg(zerxv1connect.NewFileServiceHandler(service.NewFileService(db, store, mediaResolver), opts))
	reg(zerxv1connect.NewLogServiceHandler(service.NewLogService(db), opts))
	reg(zerxv1connect.NewSiteSettingsServiceHandler(service.NewSiteSettingsService(paramCache, mediaResolver), opts))
	reg(zerxv1connect.NewDashboardServiceHandler(service.NewDashboardService(db), opts))
	pluginRoot := cfg.Plugin.ProjectRoot
	if pluginRoot == "" {
		if wd, werr := os.Getwd(); werr == nil {
			pluginRoot = wd
		}
	}
	pluginModule := modulePath(pluginRoot)
	// devGen: run `task gen` after install/uninstall only outside prod (prod
	// distroless image has no toolchain). The plugin upload accepts a binary zip,
	// so cap the request body (connect buffers it during receive, before the
	// handler's gate) to avoid OOM via a giant POST.
	pluginOpts := connect.WithOptions(opts, connect.WithReadMaxBytes(pluginUploadMaxBytes))
	reg(zerxv1connect.NewPluginServiceHandler(service.NewPluginService(
		pluginState, db, logger, cfg.UploadAllowed(), pluginRoot, pluginModule, cfg.Env != "prod",
	), pluginOpts))
	reg(zerxv1connect.NewJobServiceHandler(service.NewJobService(db, scheduler, registry, pluginState), opts))

	// Plugin handlers register through the same reg/opts, so they are mounted on
	// the API mux and covered by the full interceptor chain (incl. Casbin).
	// assertServicesRegistered then enforces every compiled zerx.v1 plugin service
	// is mounted (a missing one is fatal).
	deps := plugin.Deps{DB: db, Opts: opts, Enforcer: enforcer, Media: mediaResolver, Logger: logger}
	for _, p := range plugin.All() {
		p.RegisterHandlers(plugin.RegFunc(reg), deps)
	}

	if err := assertServicesRegistered(registered); err != nil {
		return nil, err
	}

	root := http.NewServeMux()
	root.HandleFunc("/api/export/", exportHandler(issuer, enforcer, db))
	root.HandleFunc("/api/import/users/template", importUsersTemplateHandler(issuer, enforcer))
	root.HandleFunc("/api/import/users", importUsersHandler(issuer, enforcer, db, policy))
	root.HandleFunc("/api/upload", uploadHandler(issuer, store, mediaResolver, db))
	root.Handle("/api/", http.StripPrefix("/api", api))
	if cfg.Storage.Driver == "local" {
		prefix := cfg.Storage.LocalBaseURL
		root.Handle(prefix+"/", mediaHandler(issuer, mediaResolver, db, prefix))
	}
	if cfg.Server.DocsEnabled {
		root.HandleFunc("/api/openapi.yaml", openAPIHandler())
		root.HandleFunc("/api/docs", docsHandler())
	}
	root.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		sqlDB, err := db.DB()
		if err == nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			err = sqlDB.PingContext(ctx)
		}
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("db unavailable"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
	root.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	root.Handle("/", web.SPAHandler())

	return root, nil
}

// pluginUploadMaxBytes caps the plugin install request body (source-only zip).
const pluginUploadMaxBytes = 25 << 20 // 25 MB

// modulePath reads the Go module path from go.mod at root (for plugin all.go
// import lines), falling back to the canonical module if unreadable. Keeping it
// runtime-derived makes install correct in forks created via `task new`.
func modulePath(root string) string {
	const fallback = "github.com/zerx-lab/zkit"
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return fallback
	}
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return fallback
}

// assertServicesRegistered fails if any zerx.v1 service present in the compiled
// protobuf descriptors is not mounted in the API mux — guarding against adding a
// proto service but forgetting its api.Handle registration (which 404s silently).
func assertServicesRegistered(registered []string) error {
	mounted := make(map[string]bool, len(registered))
	for _, p := range registered {
		mounted[strings.Trim(p, "/")] = true // "/zerx.v1.UserService/" -> "zerx.v1.UserService"
	}
	seen := make(map[string]bool)
	var missing []string
	for _, proc := range apispec.Procedures() {
		if seen[proc.Service] {
			continue
		}
		seen[proc.Service] = true
		if !mounted[proc.Service] {
			missing = append(missing, proc.Service)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("server: unregistered connectRPC services: %s", strings.Join(missing, ", "))
	}
	return nil
}
