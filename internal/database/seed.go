package database

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zerx-lab/zkit/internal/apispec"
	"github.com/zerx-lab/zkit/internal/model"
)

// seedMenu is the declarative source of truth for a menu node and its buttons.
type seedMenu struct {
	menu    model.Menu
	buttons []model.MenuButton
	// userVisible: granted to the built-in "user" role's menu set.
	userVisible bool
}

// crudButtons builds the standard create/update/delete button set for a resource.
func crudButtons(resource, label string) []model.MenuButton {
	return []model.MenuButton{
		{Code: resource + ":create", Name: label + "新增"},
		{Code: resource + ":update", Name: label + "编辑"},
		{Code: resource + ":delete", Name: label + "删除"},
	}
}

// seedMenus is the single source of truth for the navigation tree. Titles are
// i18n keys (translated on the client); Icon values are lucide icon names. To
// add a module: add a route file plus one entry here. After a restart syncMenus
// reconciles the entry into an existing database, so no DB reset is needed.
//
// Group headings (Path == "") have ParentID 0; their children reference the
// group by ParentID. Because IDs are assigned at insert time, parent links are
// resolved by index via parentRef below.
var seedMenuTree = []seedMenuNode{
	{node: seedMenu{menu: model.Menu{Path: "/dashboard", Name: "dashboard", Title: "nav.dashboard", Icon: "LayoutDashboardIcon", Sort: 1}, userVisible: true}},
	{node: seedMenu{menu: model.Menu{Path: "/profile", Name: "profile", Title: "nav.profile", Icon: "UserIcon", Sort: 99, Hidden: true}, userVisible: true}},
	{
		node: seedMenu{menu: model.Menu{Path: "", Name: "system", Title: "nav.system", Icon: "SettingsIcon", Sort: 10}},
		children: []seedMenuNode{
			{node: seedMenu{menu: model.Menu{Path: "/users", Name: "users", Title: "nav.users", Icon: "UsersIcon", Sort: 1}, buttons: append(crudButtons("user", "用户"), model.MenuButton{Code: "user:reset", Name: "用户重置密码"}, model.MenuButton{Code: "user:export", Name: "用户导出"}, model.MenuButton{Code: "user:import", Name: "用户导入"})}},
			{node: seedMenu{menu: model.Menu{Path: "/roles", Name: "roles", Title: "nav.roles", Icon: "ShieldIcon", Sort: 2}, buttons: crudButtons("role", "角色")}},
			{node: seedMenu{menu: model.Menu{Path: "/menus", Name: "menus", Title: "nav.menus", Icon: "ListTreeIcon", Sort: 3}, buttons: crudButtons("menu", "菜单")}},
			{node: seedMenu{menu: model.Menu{Path: "/apis", Name: "apis", Title: "nav.apis", Icon: "PlugIcon", Sort: 4}, buttons: crudButtons("api", "接口")}},
			{node: seedMenu{menu: model.Menu{Path: "/dicts", Name: "dicts", Title: "nav.dicts", Icon: "BookIcon", Sort: 5}, buttons: crudButtons("dict", "字典")}},
			{node: seedMenu{menu: model.Menu{Path: "/params", Name: "params", Title: "nav.params", Icon: "SlidersHorizontalIcon", Sort: 6}, buttons: crudButtons("param", "参数")}},
			{node: seedMenu{menu: model.Menu{Path: "/site-settings", Name: "site-settings", Title: "nav.siteSettings", Icon: "GlobeIcon", Sort: 7}, buttons: []model.MenuButton{{Code: "site:update", Name: "网站设置保存"}}}},
			{node: seedMenu{menu: model.Menu{Path: "/files", Name: "files", Title: "nav.files", Icon: "FolderIcon", Sort: 8}}},
			{node: seedMenu{menu: model.Menu{Path: "/sessions", Name: "sessions", Title: "nav.sessions", Icon: "MonitorIcon", Sort: 9}}},
			{node: seedMenu{menu: model.Menu{Path: "/jobs", Name: "jobs", Title: "nav.jobs", Icon: "ClockIcon", Sort: 10}, buttons: append(crudButtons("job", "任务"), model.MenuButton{Code: "job:run", Name: "立即执行"})}},
			{node: seedMenu{menu: model.Menu{Path: "/plugins", Name: "plugins", Title: "nav.plugins", Icon: "PuzzleIcon", Sort: 11, Component: "plugins"}, buttons: []model.MenuButton{{Code: "plugin:manage", Name: "插件启停"}}}},
		},
	},
	{
		node: seedMenu{menu: model.Menu{Path: "", Name: "audit", Title: "nav.audit", Icon: "ScrollTextIcon", Sort: 20}},
		children: []seedMenuNode{
			{node: seedMenu{menu: model.Menu{Path: "/operation-logs", Name: "operation-logs", Title: "nav.operationLogs", Icon: "ScrollTextIcon", Sort: 1}, buttons: []model.MenuButton{{Code: "operation-log:export", Name: "操作日志导出"}}}},
			{node: seedMenu{menu: model.Menu{Path: "/login-logs", Name: "login-logs", Title: "nav.loginLogs", Icon: "LogInIcon", Sort: 2}, buttons: []model.MenuButton{{Code: "login-log:export", Name: "登录日志导出"}}}},
			{node: seedMenu{menu: model.Menu{Path: "/error-logs", Name: "error-logs", Title: "nav.errorLogs", Icon: "TriangleAlertIcon", Sort: 3}, buttons: []model.MenuButton{{Code: "error-log:export", Name: "错误日志导出"}}}},
		},
	},
}

type seedMenuNode struct {
	node     seedMenu
	children []seedMenuNode
}

// MenuSeed is the exported, plugin-package-independent menu node a caller (e.g.
// cmd/server/main.go translating plugin.MenuNode) passes to Seed. It mirrors
// seedMenuNode without importing the plugin package, keeping database free of a
// database->plugin edge.
type MenuSeed struct {
	Name        string
	Path        string
	Component   string
	Title       string
	Icon        string
	Sort        int
	Hidden      bool
	UserVisible bool
	Buttons     []MenuButtonSeed
	Children    []MenuSeed
}

// MenuButtonSeed is a permission button within a MenuSeed.
type MenuButtonSeed struct {
	Code string
	Name string
}

// toSeedNodes converts exported MenuSeed nodes into the internal seedMenuNode
// tree used by insertTree/syncMenus.
func toSeedNodes(in []MenuSeed) []seedMenuNode {
	out := make([]seedMenuNode, 0, len(in))
	for i := range in {
		n := in[i]
		buttons := make([]model.MenuButton, 0, len(n.Buttons))
		for j := range n.Buttons {
			buttons = append(buttons, model.MenuButton{Code: n.Buttons[j].Code, Name: n.Buttons[j].Name})
		}
		out = append(out, seedMenuNode{
			node: seedMenu{
				menu: model.Menu{
					Path:      n.Path,
					Name:      n.Name,
					Component: n.Component,
					Title:     n.Title,
					Icon:      n.Icon,
					Sort:      n.Sort,
					Hidden:    n.Hidden,
				},
				buttons:     buttons,
				userVisible: n.UserVisible,
			},
			children: toSeedNodes(n.Children),
		})
	}
	return out
}

// ReservedMenuIdentifiers returns the Name and Path sets of the core seed menu
// tree, for plugin.ValidateAll collision checks (avoids a plugin import here).
func ReservedMenuIdentifiers() (names, paths map[string]bool) {
	names = make(map[string]bool)
	paths = make(map[string]bool)
	var walk func(nodes []seedMenuNode)
	walk = func(nodes []seedMenuNode) {
		for i := range nodes {
			m := nodes[i].node.menu
			names[m.Name] = true
			if m.Path != "" {
				paths[m.Path] = true
			}
			walk(nodes[i].children)
		}
	}
	walk(seedMenuTree)
	return names, paths
}

// Seed populates baseline RBAC data on a fresh database (idempotent on the Role
// table being empty). On an already-initialized database it incrementally
// reconciles menus and the API catalog every startup via syncMenus/syncAPIs
// (insert-only: never updates or deletes), so new entries take effect on
// restart without a DB reset. Casbin policies are NOT seeded: admin bypasses
// enforcement and the user role relies only on self-serve procedures.
func Seed(db *gorm.DB, extraMenus []MenuSeed) error {
	// Combine the core seed tree with plugin-supplied menus once; both the fresh
	// insert path and the incremental syncMenus path consume this tree.
	tree := make([]seedMenuNode, 0, len(seedMenuTree)+len(extraMenus))
	tree = append(tree, seedMenuTree...)
	tree = append(tree, toSeedNodes(extraMenus)...)

	if err := syncJobs(db); err != nil {
		return err
	}
	count, err := gorm.G[model.Role](db).Count(context.Background(), "id")
	if err != nil {
		return fmt.Errorf("seed: count roles: %w", err)
	}
	if count > 0 {
		// Roles exist already: incrementally reconcile menus + API catalog.
		if err := syncMenus(db, tree); err != nil {
			return err
		}
		return syncAPIs(db)
	}

	ctx := context.Background()

	roles := []model.Role{
		{Code: model.RoleAdmin, Name: "超级管理员", Description: "拥有全部权限", Builtin: true, Sort: 1},
		{Code: model.RoleUser, Name: "普通用户", Description: "默认角色", Builtin: true, Sort: 2},
	}
	if err := gorm.G[model.Role](db).CreateInBatches(ctx, &roles, len(roles)); err != nil {
		return fmt.Errorf("seed roles: %w", err)
	}

	var (
		adminMenuIDs   []uint64
		userMenuIDs    []uint64
		adminButtonIDs []uint64
	)

	dashboardSort := 1
	var insertTree func(nodes []seedMenuNode, parentID uint64) error
	insertTree = func(nodes []seedMenuNode, parentID uint64) error {
		for i := range nodes {
			m := nodes[i].node.menu
			m.ParentID = parentID
			if parentID == 0 && isPluginMenuName(m.Name) {
				// Plugin features sit under the dashboard, above system/audit.
				// SeedMenus.Sort is ignored for new top-level plugin rows so a
				// leftover 50 does not dump them at the bottom of the sidebar.
				m.Sort = dashboardSort + 1
			}
			if err := gorm.G[model.Menu](db).Create(ctx, &m); err != nil {
				return fmt.Errorf("seed menu %q: %w", m.Name, err)
			}
			adminMenuIDs = append(adminMenuIDs, m.ID)
			if m.Name == "dashboard" {
				dashboardSort = m.Sort
			}
			if nodes[i].node.userVisible {
				userMenuIDs = append(userMenuIDs, m.ID)
			}

			for j := range nodes[i].node.buttons {
				b := nodes[i].node.buttons[j]
				b.MenuID = m.ID
				if err := gorm.G[model.MenuButton](db).Create(ctx, &b); err != nil {
					return fmt.Errorf("seed button %q: %w", b.Code, err)
				}
				adminButtonIDs = append(adminButtonIDs, b.ID)
			}

			if err := insertTree(nodes[i].children, m.ID); err != nil {
				return err
			}
		}

		return nil
	}
	if err := insertTree(tree, 0); err != nil {
		return err
	}

	// admin: every menu + every button.
	roleMenus := make([]model.RoleMenu, 0, len(adminMenuIDs))
	for _, id := range adminMenuIDs {
		roleMenus = append(roleMenus, model.RoleMenu{RoleCode: model.RoleAdmin, MenuID: id})
	}
	// user: dashboard only.
	for _, id := range userMenuIDs {
		roleMenus = append(roleMenus, model.RoleMenu{RoleCode: model.RoleUser, MenuID: id})
	}
	if len(roleMenus) > 0 {
		if err := gorm.G[model.RoleMenu](db).CreateInBatches(ctx, &roleMenus, len(roleMenus)); err != nil {
			return fmt.Errorf("seed role menus: %w", err)
		}
	}

	roleButtons := make([]model.RoleButton, 0, len(adminButtonIDs))
	for _, id := range adminButtonIDs {
		roleButtons = append(roleButtons, model.RoleButton{RoleCode: model.RoleAdmin, ButtonID: id})
	}
	if len(roleButtons) > 0 {
		if err := gorm.G[model.RoleButton](db).CreateInBatches(ctx, &roleButtons, len(roleButtons)); err != nil {
			return fmt.Errorf("seed role buttons: %w", err)
		}
	}

	return syncAPIs(db)
}

// syncMenus additively reconciles the seed navigation tree into an existing
// database: menus are matched by Name, buttons by (menu_id, code). Missing rows
// are inserted; existing rows are never modified or deleted (admin edits and
// manually-created menus are preserved). This differs deliberately from syncAPIs,
// which prunes stale rows: menus carry RoleMenu/RoleButton associations and may
// be hand-edited by admins, so removal is left to manual operation. Newly
// inserted rows receive the same role grants a fresh seed would create (admin:
// every menu+button; user: menus flagged userVisible) so admin sees them
// immediately and the DB state matches a fresh seed. Idempotent: a second run
// inserts nothing.
func syncMenus(db *gorm.DB, tree []seedMenuNode) error {
	ctx := context.Background()

	existing, err := gorm.G[model.Menu](db).Find(ctx)
	if err != nil {
		return fmt.Errorf("sync menus: load menus: %w", err)
	}
	byName := make(map[string]uint64, len(existing))
	dashboardSort := 1
	for i := range existing {
		byName[existing[i].Name] = existing[i].ID
		if existing[i].Name == "dashboard" {
			dashboardSort = existing[i].Sort
		}
	}

	existingButtons, err := gorm.G[model.MenuButton](db).Find(ctx)
	if err != nil {
		return fmt.Errorf("sync menus: load buttons: %w", err)
	}
	haveButton := make(map[string]bool, len(existingButtons))
	for i := range existingButtons {
		haveButton[buttonKey(existingButtons[i].MenuID, existingButtons[i].Code)] = true
	}

	var walk func(nodes []seedMenuNode, parentName string) error
	walk = func(nodes []seedMenuNode, parentName string) error {
		for i := range nodes {
			seed := nodes[i].node
			menuID, ok := byName[seed.menu.Name]
			if !ok {
				m := seed.menu
				if parentName != "" {
					m.ParentID = byName[parentName] // parent walked first; always present
				} else if isPluginMenuName(m.Name) {
					m.Sort = dashboardSort + 1
				}
				if err := gorm.G[model.Menu](db).Create(ctx, &m); err != nil {
					return fmt.Errorf("sync menu %q: %w", m.Name, err)
				}
				menuID = m.ID
				byName[m.Name] = menuID

				if err := gorm.G[model.RoleMenu](db).Create(ctx, &model.RoleMenu{RoleCode: model.RoleAdmin, MenuID: menuID}); err != nil {
					return fmt.Errorf("sync menu grant admin %q: %w", m.Name, err)
				}
				if seed.userVisible {
					if err := gorm.G[model.RoleMenu](db).Create(ctx, &model.RoleMenu{RoleCode: model.RoleUser, MenuID: menuID}); err != nil {
						return fmt.Errorf("sync menu grant user %q: %w", m.Name, err)
					}
				}
			} else if isPluginMenuName(seed.menu.Name) {
				// Plugin menus are reconciled (not insert-only): path/component/
				// parent/title/icon/hidden stay owned by SeedMenus so a flat page
				// becoming a grouped sub-page across versions is not left stale.
				// Sort is NOT overwritten — admin drag-reorder / UpdateMenu persist
				// across restarts. Core menus stay insert-only above.
				var parentID uint64
				if parentName != "" {
					parentID = byName[parentName]
				}
				if err := db.WithContext(ctx).Model(&model.Menu{}).Where("id = ?", menuID).Updates(map[string]any{
					"parent_id": parentID,
					"path":      seed.menu.Path,
					"component": seed.menu.Component,
					"title":     seed.menu.Title,
					"icon":      seed.menu.Icon,
					"hidden":    seed.menu.Hidden,
				}).Error; err != nil {
					return fmt.Errorf("sync menu reconcile %q: %w", seed.menu.Name, err)
				}
			}

			for j := range seed.buttons {
				b := seed.buttons[j]
				if haveButton[buttonKey(menuID, b.Code)] {
					continue
				}
				b.MenuID = menuID
				if err := gorm.G[model.MenuButton](db).Create(ctx, &b); err != nil {
					return fmt.Errorf("sync button %q: %w", b.Code, err)
				}
				haveButton[buttonKey(menuID, b.Code)] = true
				if err := gorm.G[model.RoleButton](db).Create(ctx, &model.RoleButton{RoleCode: model.RoleAdmin, ButtonID: b.ID}); err != nil {
					return fmt.Errorf("sync button grant admin %q: %w", b.Code, err)
				}
			}

			if err := walk(nodes[i].children, seed.menu.Name); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(tree, ""); err != nil {
		return err
	}

	return pruneOrphanPluginMenus(db, tree)
}

// pruneOrphanPluginMenus removes menus whose Name is plugin-namespaced (plg_*)
// but is no longer declared by any registered plugin (its node is absent from
// the combined seed tree). This auto-cleans the sidebar after a plugin is
// removed + rebuilt, so a stale "plg_<gone>" entry doesn't linger and render
// NotFound. Only plugin-namespaced menus are touched — core menus and
// admin-created menus (non-plg_ names) are never pruned. Owned data tables and
// apis rows are NOT touched here (manual teardown.sql handles those, to avoid
// deleting business data on a transient mis-registration).
func pruneOrphanPluginMenus(db *gorm.DB, tree []seedMenuNode) error {
	ctx := context.Background()

	// Names declared by the current (core + registered-plugin) seed tree.
	known := make(map[string]bool)
	var collect func(nodes []seedMenuNode)
	collect = func(nodes []seedMenuNode) {
		for i := range nodes {
			known[nodes[i].node.menu.Name] = true
			collect(nodes[i].children)
		}
	}
	collect(tree)

	existing, err := gorm.G[model.Menu](db).Find(ctx)
	if err != nil {
		return fmt.Errorf("prune menus: load: %w", err)
	}
	var orphanIDs []uint64
	for i := range existing {
		if isPluginMenuName(existing[i].Name) && !known[existing[i].Name] {
			orphanIDs = append(orphanIDs, existing[i].ID)
		}
	}
	if len(orphanIDs) == 0 {
		return nil
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Remove button grants, then buttons, then menu grants, then the menus.
		if err := tx.Where("button_id IN (?)",
			tx.Model(&model.MenuButton{}).Select("id").Where("menu_id IN ?", orphanIDs),
		).Delete(&model.RoleButton{}).Error; err != nil {
			return err
		}
		if err := tx.Where("menu_id IN ?", orphanIDs).Delete(&model.MenuButton{}).Error; err != nil {
			return err
		}
		if err := tx.Where("menu_id IN ?", orphanIDs).Delete(&model.RoleMenu{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ?", orphanIDs).Delete(&model.Menu{}).Error; err != nil {
			return err
		}
		return nil
	})
}

// isPluginMenuName reports whether a menu Name belongs to a plugin (namespaced
// "plg_*"). Plugin menus are reconciled by syncMenus; core menus are insert-only.
func isPluginMenuName(name string) bool {
	return strings.HasPrefix(name, "plg_")
}

func buttonKey(menuID uint64, code string) string {
	return fmt.Sprintf("%d|%s", menuID, code)
}

// syncAPIs upserts the full procedure catalog into the apis table.
func syncAPIs(db *gorm.DB) error {
	procs := apispec.Procedures()
	if len(procs) == 0 {
		return nil
	}

	rows := make([]model.API, 0, len(procs))
	for _, p := range procs {
		rows = append(rows, model.API{
			Procedure: p.Procedure,
			Service:   p.Service,
			Method:    p.Method,
			Group:     shortService(p.Service),
		})
	}

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "procedure"}},
		DoNothing: true,
	}).Create(&rows).Error
}

// shortService returns the trailing segment of a fully-qualified service name
// (zerx.v1.UserService -> UserService).
func shortService(full string) string {
	for i := len(full) - 1; i >= 0; i-- {
		if full[i] == '.' {
			return full[i+1:]
		}
	}

	return full
}

// syncJobs idempotently inserts the built-in scheduled jobs (matched by Name).
// Runs on every startup before the role-seed branch so existing databases also
// receive new default jobs. Existing rows are never modified.
func syncJobs(db *gorm.DB) error {
	ctx := context.Background()
	defaults := []model.ScheduledJob{
		{Name: "每日日志清理", Handler: "log_cleanup", CronExpr: "0 3 * * *", Enabled: true, Description: "清理 30 天前的操作日志与登录日志"},
		{Name: "每日会话清理", Handler: "session_cleanup", CronExpr: "30 3 * * *", Enabled: true, Description: "清理已过期的用户会话"},
		{Name: "认证状态清理", Handler: "auth_state_cleanup", CronExpr: "15 3 * * *", Enabled: true, Description: "清理过期的验证码与登录失败记录"},
	}
	for i := range defaults {
		n, err := gorm.G[model.ScheduledJob](db).Where("name = ?", defaults[i].Name).Count(ctx, "id")
		if err != nil {
			return fmt.Errorf("sync jobs: count %q: %w", defaults[i].Name, err)
		}
		if n > 0 {
			continue
		}
		if err := gorm.G[model.ScheduledJob](db).Create(ctx, &defaults[i]); err != nil {
			return fmt.Errorf("sync jobs: create %q: %w", defaults[i].Name, err)
		}
	}

	return nil
}
