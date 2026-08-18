package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/audit"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/plugin"
	"github.com/zerx-lab/zkit/internal/plugin/installer"
)

// PluginService exposes the compiled-in plugin registry, runtime enable/disable
// state, and scaffold-style install/uninstall of plugin source packages.
//
// Compile-time model: install/uninstall write source to disk and edit all.go but
// do NOT run new code in the current process; a rebuild + restart applies them
// (air auto-restarts in dev). Install/uninstall are admin-only AND gated by
// uploadAllowed (off in prod by default) since they write executable source.
type PluginService struct {
	state         *plugin.State
	db            *gorm.DB
	logger        *slog.Logger
	uploadAllowed bool
	projectRoot   string // repo root to write into
	module        string // Go module path (for all.go import lines)
	devGen        bool   // run `task gen` after install/uninstall (dev only)
}

var _ zerxv1connect.PluginServiceHandler = (*PluginService)(nil)

// NewPluginService constructs the handler. uploadAllowed/projectRoot/module/
// devGen configure install/uninstall; a zero PluginConfig disables uploads.
func NewPluginService(state *plugin.State, db *gorm.DB, logger *slog.Logger, uploadAllowed bool, projectRoot, module string, devGen bool) *PluginService {
	return &PluginService{
		state:         state,
		db:            db,
		logger:        logger,
		uploadAllowed: uploadAllowed,
		projectRoot:   projectRoot,
		module:        module,
		devGen:        devGen,
	}
}

func (s *PluginService) ListPlugins(_ context.Context, _ *connect.Request[zerxv1.ListPluginsRequest]) (*connect.Response[zerxv1.ListPluginsResponse], error) {
	plugins := plugin.All()
	out := make([]*zerxv1.PluginInfo, 0, len(plugins))
	for _, p := range plugins {
		out = append(out, s.toInfo(p))
	}
	return connect.NewResponse(&zerxv1.ListPluginsResponse{Plugins: out, UploadAllowed: s.uploadAllowed}), nil
}

// ListPublicPages is anonymous: it returns the public pages of enabled plugins
// for the unauthenticated /pub/$ route to resolve. Disabled plugins' pages are
// omitted (their route renders NotFound).
func (s *PluginService) ListPublicPages(_ context.Context, _ *connect.Request[zerxv1.ListPublicPagesRequest]) (*connect.Response[zerxv1.ListPublicPagesResponse], error) {
	pages := s.state.EnabledPublicPages()
	out := make([]*zerxv1.PluginPublicPage, 0, len(pages))
	for i := range pages {
		out = append(out, &zerxv1.PluginPublicPage{Path: pages[i].Path, Component: pages[i].Component, Title: pages[i].Title})
	}
	return connect.NewResponse(&zerxv1.ListPublicPagesResponse{Pages: out}), nil
}

// toInfo builds the admin-facing PluginInfo with full detail (menus + pages).
func (s *PluginService) toInfo(p plugin.Plugin) *zerxv1.PluginInfo {
	jobs := make([]string, 0, len(p.JobHandlers()))
	for k := range p.JobHandlers() {
		jobs = append(jobs, k)
	}
	var menus []*zerxv1.PluginMenu
	var collect func(nodes []plugin.MenuNode)
	collect = func(nodes []plugin.MenuNode) {
		for i := range nodes {
			menus = append(menus, &zerxv1.PluginMenu{Name: nodes[i].Name, Path: nodes[i].Path, Title: nodes[i].Title})
			collect(nodes[i].Children)
		}
	}
	collect(p.SeedMenus())
	pages := make([]*zerxv1.PluginPublicPage, 0, len(p.PublicPages()))
	for _, pg := range p.PublicPages() {
		pages = append(pages, &zerxv1.PluginPublicPage{Path: pg.Path, Component: pg.Component, Title: pg.Title})
	}
	return &zerxv1.PluginInfo{
		Name:           p.Name(),
		Enabled:        s.state.IsEnabled(p.Name()),
		PendingRemoval: s.sourceMissing(p.Name()),
		Services:       p.Services(),
		Tables:         p.TableNames(),
		MigrationCount: int32(len(p.Migrations())),
		MenuCount:      int32(countMenuNodes(p.SeedMenus())),
		JobHandlers:    jobs,
		Menus:          menus,
		PublicPages:    pages,
	}
}

func (s *PluginService) SetPluginEnabled(ctx context.Context, req *connect.Request[zerxv1.SetPluginEnabledRequest]) (*connect.Response[zerxv1.PluginInfo], error) {
	name := req.Msg.GetName()
	var target plugin.Plugin
	for _, p := range plugin.All() {
		if p.Name() == name {
			target = p
			break
		}
	}
	if target == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("unknown plugin: "+name))
	}

	if err := s.state.SetEnabled(ctx, name, req.Msg.GetEnabled()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"name": name, "enabled": req.Msg.GetEnabled()}}))

	return connect.NewResponse(s.toInfo(target)), nil
}

// InstallPlugin unpacks an uploaded plugin source package and wires it into
// all.go. Admin-only (Casbin) + gated by uploadAllowed. Effect requires a
// rebuild/restart (auto via air in dev).
func (s *PluginService) InstallPlugin(ctx context.Context, req *connect.Request[zerxv1.InstallPluginRequest]) (*connect.Response[zerxv1.InstallPluginResponse], error) {
	if !s.uploadAllowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("plugin upload is disabled (set PLUGIN_UPLOAD_ENABLED=true to allow)"))
	}
	res, err := installer.Install(req.Msg.GetPackage(), s.projectRoot, s.module)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	// Record audit BEFORE kicking off gen: the async gen may trigger an air
	// rebuild that restarts this process.
	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"installed": res.Name}}))
	s.maybeGen()
	return connect.NewResponse(&zerxv1.InstallPluginResponse{
		Name:           res.Name,
		PendingRestart: true,
		Message:        pendingCode(s.devGen),
	}), nil
}

// UninstallPlugin removes a plugin's source + all.go lines, optionally purging
// its data. Admin-only + gated by uploadAllowed. Effect requires rebuild/restart.
func (s *PluginService) UninstallPlugin(ctx context.Context, req *connect.Request[zerxv1.UninstallPluginRequest]) (*connect.Response[zerxv1.UninstallPluginResponse], error) {
	if !s.uploadAllowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("plugin upload is disabled (set PLUGIN_UPLOAD_ENABLED=true to allow)"))
	}
	name := req.Msg.GetName()
	// Purge data (tables/apis/casbin/jobs/migrations/plugin_states) BEFORE
	// removing source, while we can still introspect; menus self-prune on restart.
	if req.Msg.GetPurgeData() {
		if err := s.teardownData(ctx, name); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	if _, err := installer.Uninstall(s.projectRoot, s.module, name); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"uninstalled": name, "purged": req.Msg.GetPurgeData()}}))
	s.maybeGen()
	return connect.NewResponse(&zerxv1.UninstallPluginResponse{
		PendingRestart: true,
		Message:        pendingCode(s.devGen),
	}), nil
}

// maybeGen runs `task gen` after a source change in dev so air rebuilds with
// generated code. It runs ASYNCHRONOUSLY in a detached background context: (1)
// the install/uninstall RPC returns immediately (task gen takes ~tens of
// seconds; blocking the request would look "unresponsive"), and (2) the detached
// context means air restarting this process (triggered by the very source change
// we just made) does not cancel the request ctx and kill the response. Files are
// already on disk when we return, so the operation has already succeeded; gen +
// rebuild is the eventual-consistency step. Best-effort; prod has no toolchain.
func (s *PluginService) maybeGen() {
	if !s.devGen {
		return
	}
	go func() {
		cmd := exec.CommandContext(context.Background(), "task", "gen")
		cmd.Dir = s.projectRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			s.logger.Warn("plugin: task gen failed (run it manually)", "err", err, "out", string(out))
		}
	}()
}

// pendingCode returns a locale-independent code the client localizes
// (pluginPage.pendingDev / pluginPage.pendingProd), avoiding a server-side
// hardcoded-language string in the toast.
func pendingCode(devGen bool) string {
	if devGen {
		return "dev"
	}
	return "prod"
}

// teardownData removes a plugin's persisted residue. It runs BEFORE the source
// is removed, so the plugin is still in plugin.All() and we can DROP its actual
// declared tables. Uses GORM model deletes (per-dialect identifier quoting, so
// the reserved `procedure` column is safe) and ESCAPE '!' (not special in
// pg/mysql/sqlite string literals, unlike backslash on MySQL). Best-effort: a
// failing step is logged, the rest continue.
func (s *PluginService) teardownData(ctx context.Context, name string) error {
	db := s.db.WithContext(ctx)
	groupName := "plg_" + name
	menuLike := "plg!_" + name + "!_%"
	jobLike := name + "!_%"
	procLike := "/zerx.v1." + pascalCaseSvc(name) + "%"
	migLike := "plg!_" + name + "!_%"

	log := func(step string, err error) {
		if err != nil {
			s.logger.Warn("plugin teardown step failed", "name", name, "step", step, "err", err)
		}
	}

	// 0. DROP the plugin's own data tables (registry still intact at this point),
	// so a same-name reinstall starts clean rather than resurfacing old rows.
	for _, p := range plugin.All() {
		if p.Name() != name {
			continue
		}
		for _, tbl := range p.TableNames() {
			log("drop "+tbl, db.Migrator().DropTable(tbl))
		}
	}

	// 1. Menus + grants (child rows first), via GORM subqueries. Unscoped() forces
	// HARD deletes: soft-deleted (deleted_at) rows would otherwise persist and
	// collide with a same-name reinstall on the menu Name unique index.
	menuIDs := db.Unscoped().Model(&model.Menu{}).Select("id").Where("name = ? OR name LIKE ? ESCAPE '!'", groupName, menuLike)
	btnIDs := db.Unscoped().Model(&model.MenuButton{}).Select("id").Where("menu_id IN (?)", menuIDs)
	log("role_buttons", db.Unscoped().Where("button_id IN (?)", btnIDs).Delete(&model.RoleButton{}).Error)
	log("menu_buttons", db.Unscoped().Where("menu_id IN (?)", menuIDs).Delete(&model.MenuButton{}).Error)
	log("role_menus", db.Unscoped().Where("menu_id IN (?)", menuIDs).Delete(&model.RoleMenu{}).Error)
	log("menus", db.Unscoped().Where("name = ? OR name LIKE ? ESCAPE '!'", groupName, menuLike).Delete(&model.Menu{}).Error)

	// 2. Scheduled jobs + executions.
	jobIDs := db.Unscoped().Model(&model.ScheduledJob{}).Select("id").Where("handler LIKE ? ESCAPE '!'", jobLike)
	log("job_executions", db.Unscoped().Where("job_id IN (?)", jobIDs).Delete(&model.JobExecution{}).Error)
	log("scheduled_jobs", db.Unscoped().Where("handler LIKE ? ESCAPE '!'", jobLike).Delete(&model.ScheduledJob{}).Error)

	// 3. API catalog (model delete quotes the reserved `procedure` column).
	log("apis", db.Unscoped().Where("procedure LIKE ?", procLike).Delete(&model.API{}).Error)

	// 4. Casbin policies for the plugin's procedures.
	log("casbin_rule", db.Exec("DELETE FROM casbin_rule WHERE v1 LIKE ?", procLike).Error)

	// 5. Migration ledger (after dropping tables, so reinstall recreates them).
	log("migrations", db.Exec("DELETE FROM migrations WHERE id LIKE ? ESCAPE '!'", migLike).Error)

	// 6. Runtime enable/disable state.
	log("plugin_states", db.Where("name = ?", name).Delete(&model.PluginState{}).Error)
	return nil
}

// pascalCaseSvc converts a plugin name to its service PascalCase prefix
// ("shop" -> "Shop"), matching the validate.go service-naming rule.
func pascalCaseSvc(name string) string {
	var b []rune
	upper := true
	for _, r := range name {
		if r == '_' {
			upper = true
			continue
		}
		if upper && r >= 'a' && r <= 'z' {
			r -= 32
		}
		upper = false
		b = append(b, r)
	}
	return string(b)
}

// sourceMissing reports whether a compiled-in plugin's source directory is gone
// from disk (i.e. it was uninstalled and awaits a rebuild/restart to actually
// disappear from the running binary). projectRoot empty => never report pending.
func (s *PluginService) sourceMissing(name string) bool {
	if s.projectRoot == "" {
		return false
	}
	implDir := filepath.Join(s.projectRoot, "internal", "plugin", "impl", name)
	if _, err := os.Stat(implDir); err != nil {
		return os.IsNotExist(err)
	}
	return false
}

// countMenuNodes counts all nodes in a plugin menu subtree (incl. children).
func countMenuNodes(nodes []plugin.MenuNode) int {
	n := 0
	for i := range nodes {
		n++
		n += countMenuNodes(nodes[i].Children)
	}
	return n
}
