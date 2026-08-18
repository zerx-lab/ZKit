// Package plugin defines the compile-time plugin contract for ZKit.
//
// A plugin bundles everything needed to add a back-office module without
// touching the core wiring: its connectRPC handlers, GORM migrations, seed
// menus, public/self-serve procedure declarations, and optional background
// jobs. Plugins are registered explicitly (not via init() magic) in
// internal/plugins/all.go and validated at startup by ValidateAll.
//
// Cycle-avoidance contract (load-bearing): this package never imports
// internal/jobs (job handlers use the local JobHandler type) and is never
// imported by internal/database (Migrate/Seed take plain slices assembled in
// cmd/server/main.go). Breaking either edge would close the
// database -> plugin -> jobs import cycle that the jobs test package relies on.
package plugin

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/casbin/casbin/v3"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/media"
)

// RegFunc mirrors the reg closure in internal/server/server.go: it records the
// handler path for assertServicesRegistered and mounts it on the API mux.
type RegFunc func(path string, h http.Handler)

// JobHandler is a plugin-owned background task. It deliberately does not
// reference jobs.Descriptor so the plugin package never imports internal/jobs;
// cmd/server/main.go converts it into a jobs.Descriptor at assembly time.
type JobHandler struct {
	Run         func(ctx context.Context) error
	Description string
}

// Deps carries the shared core dependencies handed to a plugin's
// RegisterHandlers. The field set is canonical (all five fields always
// present); a plugin simply ignores the ones it does not need.
type Deps struct {
	DB       *gorm.DB
	Opts     connect.Option // = server.go opts (full interceptor chain)
	Enforcer *casbin.SyncedCachedEnforcer
	Media    *media.Media
	Logger   *slog.Logger
}

// MenuNode is the exported, plugin-package-local mirror of the database
// package's private seedMenuNode. A plugin declares its navigation subtree with
// these; cmd/server/main.go converts them to database.MenuSeed before seeding.
type MenuNode struct {
	Name      string // must be prefixed "plg_<plugin>"
	Path      string // must be prefixed "/p/<plugin>" (leaf routes); "" for a group heading
	Component string // glob-relative component id, e.g. "shop/Shop"
	Title     string // i18n key
	Icon      string // lucide icon name
	Sort      int
	Hidden    bool
	// UserVisible grants the node to the built-in "user" role's menu set on
	// insert (mirrors seedMenuNode.userVisible).
	UserVisible bool
	Buttons     []MenuButton
	Children    []MenuNode
}

// MenuButton is a permission button under a menu node.
type MenuButton struct {
	Code string
	Name string
}

// PublicPage is an anonymous-accessible (no auth) front-end page a plugin serves
// under /pub. Use it for public-facing modules (e.g. a marketing site) that need
// a route + optional public API but no admin login. Path must be namespaced
// "/pub/<plugin>" or "/pub/<plugin>/..."; Component is the glob id under
// web/src/plugin-components (e.g. "site/Landing").
type PublicPage struct {
	Path      string
	Component string
	Title     string // i18n key (optional, for document title)
}

// Plugin is the contract every plugin implements. All method return/param types
// avoid packages that would form an import cycle (see package doc).
type Plugin interface {
	// Name is the plugin identifier: lower snake_case, regex
	// ^[a-z][a-z0-9_]{0,30}$, globally unique.
	Name() string

	// Services returns the fully-qualified connectRPC service names this plugin
	// owns (e.g. "zerx.v1.ShopProductService"). ValidateAll uses these to verify
	// every Public/SelfServe procedure belongs to this plugin.
	Services() []string

	// Migrations returns gormigrate migrations; each ID must be prefixed
	// "plg_<name>_".
	Migrations() []*gormigrate.Migration

	// TableNames returns every table this plugin owns; each must be prefixed
	// "plg_<name>_". ValidateAll checks these to block AutoMigrate polluting core
	// tables.
	TableNames() []string

	// SeedMenus returns the navigation subtree; Name/Path namespaced per the
	// MenuNode doc.
	SeedMenus() []MenuNode

	// PublicProcedures returns procedures callable without authentication. Each
	// must belong to one of Services().
	PublicProcedures() []string

	// SelfServeProcedures returns procedures any authenticated caller may invoke
	// (no Casbin check). Each must belong to one of Services().
	SelfServeProcedures() []string

	// RegisterHandlers mounts the plugin's connectRPC handlers via reg, using the
	// shared deps (notably deps.Opts for the full interceptor chain).
	RegisterHandlers(reg RegFunc, deps Deps)

	// JobHandlers returns optional background jobs; each key must be prefixed
	// "<name>_".
	JobHandlers() map[string]JobHandler

	// PublicPages returns anonymous-accessible front-end pages served under /pub.
	// Empty for back-office-only plugins. Paths must be namespaced "/pub/<name>".
	PublicPages() []PublicPage
}
