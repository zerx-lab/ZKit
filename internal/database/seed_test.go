package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/model"
)

func newSeededDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := Migrate(db, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := Seed(db, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

func countMenusByName(t *testing.T, db *gorm.DB, name string) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.Menu{}).Where("name = ?", name).Count(&n).Error; err != nil {
		t.Fatalf("count menu %q: %v", name, err)
	}
	return n
}

func TestSyncMenusReinsertsMissingAndIsIdempotent(t *testing.T) {
	db := newSeededDB(t)

	// Simulate a menu added to seedMenuTree after the DB was initialized.
	if err := db.Unscoped().Where("name = ?", "files").Delete(&model.Menu{}).Error; err != nil {
		t.Fatalf("delete files menu: %v", err)
	}
	if countMenusByName(t, db, "files") != 0 {
		t.Fatal("files menu not deleted")
	}

	if err := syncMenus(db, seedMenuTree); err != nil {
		t.Fatalf("syncMenus: %v", err)
	}

	var files model.Menu
	if err := db.Where("name = ?", "files").First(&files).Error; err != nil {
		t.Fatalf("files menu missing after sync: %v", err)
	}
	var system model.Menu
	if err := db.Where("name = ?", "system").First(&system).Error; err != nil {
		t.Fatalf("system menu missing: %v", err)
	}
	if files.ParentID != system.ID {
		t.Fatalf("files ParentID = %d, want system ID %d", files.ParentID, system.ID)
	}
	var grants int64
	if err := db.Model(&model.RoleMenu{}).Where("role_code = ? AND menu_id = ?", model.RoleAdmin, files.ID).Count(&grants).Error; err != nil {
		t.Fatalf("count admin grant: %v", err)
	}
	if grants != 1 {
		t.Fatalf("admin RoleMenu count = %d, want 1", grants)
	}

	// Idempotent: a second run inserts nothing.
	if err := syncMenus(db, seedMenuTree); err != nil {
		t.Fatalf("syncMenus second run: %v", err)
	}
	if got := countMenusByName(t, db, "files"); got != 1 {
		t.Fatalf("files menu count after second sync = %d, want 1", got)
	}
}

func TestSyncMenusReinsertsMissingButton(t *testing.T) {
	db := newSeededDB(t)

	if err := db.Unscoped().Where("code = ?", "user:reset").Delete(&model.MenuButton{}).Error; err != nil {
		t.Fatalf("delete button: %v", err)
	}

	if err := syncMenus(db, seedMenuTree); err != nil {
		t.Fatalf("syncMenus: %v", err)
	}

	var btn model.MenuButton
	if err := db.Where("code = ?", "user:reset").First(&btn).Error; err != nil {
		t.Fatalf("button missing after sync: %v", err)
	}
	var grants int64
	if err := db.Model(&model.RoleButton{}).Where("role_code = ? AND button_id = ?", model.RoleAdmin, btn.ID).Count(&grants).Error; err != nil {
		t.Fatalf("count admin grant: %v", err)
	}
	if grants != 1 {
		t.Fatalf("admin RoleButton count = %d, want 1", grants)
	}
}

func TestSyncMenusPrunesOrphanPluginMenus(t *testing.T) {
	db := newSeededDB(t)

	// A plugin menu left over from an uninstalled plugin (not in the seed tree),
	// with a button + role grants, plus an admin-created non-plugin menu.
	orphan := model.Menu{Name: "plg_gone_page", Path: "/p/gone", Component: "gone/X"}
	if err := db.Create(&orphan).Error; err != nil {
		t.Fatalf("create orphan: %v", err)
	}
	btn := model.MenuButton{MenuID: orphan.ID, Code: "plg_gone:do", Name: "x"}
	if err := db.Create(&btn).Error; err != nil {
		t.Fatalf("create orphan button: %v", err)
	}
	if err := db.Create(&model.RoleMenu{RoleCode: model.RoleAdmin, MenuID: orphan.ID}).Error; err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := db.Create(&model.RoleButton{RoleCode: model.RoleAdmin, ButtonID: btn.ID}).Error; err != nil {
		t.Fatalf("grant btn: %v", err)
	}
	adminMade := model.Menu{Name: "custom-report", Path: "/custom", Component: "Custom"}
	if err := db.Create(&adminMade).Error; err != nil {
		t.Fatalf("create admin menu: %v", err)
	}

	if err := syncMenus(db, seedMenuTree); err != nil {
		t.Fatalf("syncMenus: %v", err)
	}

	// Orphan plugin menu + its associations are gone.
	if countMenusByName(t, db, "plg_gone_page") != 0 {
		t.Fatal("orphan plugin menu not pruned")
	}
	var nBtn, nRM, nRB int64
	db.Model(&model.MenuButton{}).Where("menu_id = ?", orphan.ID).Count(&nBtn)
	db.Model(&model.RoleMenu{}).Where("menu_id = ?", orphan.ID).Count(&nRM)
	db.Model(&model.RoleButton{}).Where("button_id = ?", btn.ID).Count(&nRB)
	if nBtn != 0 || nRM != 0 || nRB != 0 {
		t.Fatalf("orphan associations not pruned: btn=%d rm=%d rb=%d", nBtn, nRM, nRB)
	}
	// Admin-created non-plugin menu and core menus survive.
	if countMenusByName(t, db, "custom-report") != 1 {
		t.Fatal("admin-created menu must not be pruned")
	}
	if countMenusByName(t, db, "users") != 1 {
		t.Fatal("core menu must not be pruned")
	}
}

func TestSyncMenusReconcilesPluginMenuLayout(t *testing.T) {
	db := newSeededDB(t)

	// Simulate a stale flat plugin menu from a previous version.
	stale := model.Menu{Name: "plg_demo_page", Path: "/p/demo", Component: "demo/Old", Title: "plg.demo.x", Sort: 9}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatalf("create stale plugin menu: %v", err)
	}

	// New version moves it under a group with a new path/component.
	tree := append([]seedMenuNode{}, seedMenuTree...)
	tree = append(tree, seedMenuNode{
		node: seedMenu{menu: model.Menu{Name: "plg_demo", Path: "", Title: "plg.demo.group", Sort: 60}},
		children: []seedMenuNode{
			{node: seedMenu{menu: model.Menu{Name: "plg_demo_page", Path: "/p/demo/page", Component: "demo/Page", Title: "plg.demo.page", Sort: 1}}},
		},
	})

	if err := syncMenus(db, tree); err != nil {
		t.Fatalf("syncMenus: %v", err)
	}

	var group model.Menu
	if err := db.Where("name = ?", "plg_demo").First(&group).Error; err != nil {
		t.Fatalf("group menu missing: %v", err)
	}
	var page model.Menu
	if err := db.Where("name = ?", "plg_demo_page").First(&page).Error; err != nil {
		t.Fatalf("page menu missing: %v", err)
	}
	// The stale plugin menu must be reconciled to the new layout.
	if page.Path != "/p/demo/page" || page.Component != "demo/Page" || page.ParentID != group.ID {
		t.Fatalf("plugin menu not reconciled: path=%q component=%q parent=%d (want /p/demo/page, demo/Page, %d)", page.Path, page.Component, page.ParentID, group.ID)
	}
	// Sort is admin-owned after insert: the stale page's 9 must survive seed Sort 1.
	if page.Sort != 9 {
		t.Fatalf("plugin page Sort = %d, want 9 (sync must not overwrite sort)", page.Sort)
	}
}

func TestSeedPlacesPluginMenuAfterDashboard(t *testing.T) {
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := Migrate(db, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	extra := []MenuSeed{{
		Name: "plg_demo", Path: "", Title: "plg.demo", Icon: "CircleIcon", Sort: 50,
	}}
	if err := Seed(db, extra); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var menus []model.Menu
	if err := db.Where("parent_id = ?", 0).Order("sort ASC, id ASC").Find(&menus).Error; err != nil {
		t.Fatalf("list top menus: %v", err)
	}
	var visible []string
	for i := range menus {
		if !menus[i].Hidden {
			visible = append(visible, menus[i].Name)
		}
	}
	want := []string{"dashboard", "plg_demo", "system", "audit"}
	if len(visible) != len(want) {
		t.Fatalf("visible top menus = %v, want %v", visible, want)
	}
	for i := range want {
		if visible[i] != want[i] {
			t.Fatalf("visible top menus = %v, want %v", visible, want)
		}
	}
}

func TestSyncMenusInsertsPluginAfterDashboard(t *testing.T) {
	db := newSeededDB(t)

	tree := append([]seedMenuNode{}, seedMenuTree...)
	tree = append(tree, seedMenuNode{
		node: seedMenu{menu: model.Menu{Name: "plg_demo", Path: "", Title: "plg.demo", Sort: 50}},
	})
	if err := syncMenus(db, tree); err != nil {
		t.Fatalf("syncMenus: %v", err)
	}

	var dash, plugin, system model.Menu
	if err := db.Where("name = ?", "dashboard").First(&dash).Error; err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if err := db.Where("name = ?", "plg_demo").First(&plugin).Error; err != nil {
		t.Fatalf("plugin: %v", err)
	}
	if err := db.Where("name = ?", "system").First(&system).Error; err != nil {
		t.Fatalf("system: %v", err)
	}
	if plugin.Sort != dash.Sort+1 {
		t.Fatalf("plugin Sort = %d, want dashboard+1 (%d)", plugin.Sort, dash.Sort+1)
	}
	if plugin.Sort >= system.Sort {
		t.Fatalf("plugin Sort %d should be < system Sort %d", plugin.Sort, system.Sort)
	}
}

func TestSyncMenusPreservesPluginSort(t *testing.T) {
	db := newSeededDB(t)

	existing := model.Menu{Name: "plg_demo", Path: "", Title: "plg.demo", Sort: 7}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create plugin menu: %v", err)
	}

	tree := append([]seedMenuNode{}, seedMenuTree...)
	tree = append(tree, seedMenuNode{
		node: seedMenu{menu: model.Menu{Name: "plg_demo", Path: "", Title: "plg.demo.updated", Sort: 50}},
	})
	if err := syncMenus(db, tree); err != nil {
		t.Fatalf("syncMenus: %v", err)
	}

	var got model.Menu
	if err := db.Where("name = ?", "plg_demo").First(&got).Error; err != nil {
		t.Fatalf("load plugin menu: %v", err)
	}
	if got.Sort != 7 {
		t.Fatalf("plugin Sort = %d, want 7 (admin order must persist)", got.Sort)
	}
	if got.Title != "plg.demo.updated" {
		t.Fatalf("plugin Title = %q, want plg.demo.updated (layout still syncs)", got.Title)
	}
}

func TestSyncMenusPreservesExistingEdits(t *testing.T) {
	db := newSeededDB(t)

	if err := db.Model(&model.Menu{}).Where("name = ?", "users").Update("title", "custom.title").Error; err != nil {
		t.Fatalf("edit menu: %v", err)
	}

	if err := syncMenus(db, seedMenuTree); err != nil {
		t.Fatalf("syncMenus: %v", err)
	}

	var users model.Menu
	if err := db.Where("name = ?", "users").First(&users).Error; err != nil {
		t.Fatalf("users menu missing: %v", err)
	}
	if users.Title != "custom.title" {
		t.Fatalf("users Title = %q, want custom.title (sync must not overwrite)", users.Title)
	}
}
