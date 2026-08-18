package service

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	casbinlib "github.com/casbin/casbin/v3"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/internal/auth"
	casbinpkg "github.com/zerx-lab/zkit/internal/casbin"
	"github.com/zerx-lab/zkit/internal/database"
	"github.com/zerx-lab/zkit/internal/model"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(db, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := database.Seed(db, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

func newEnforcer(t *testing.T, db *gorm.DB) *casbinlib.SyncedCachedEnforcer {
	t.Helper()
	e, err := casbinpkg.New(db)
	if err != nil {
		t.Fatalf("casbin: %v", err)
	}
	return e
}

func TestRoleLifecycle(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)
	svc := NewRoleService(db, e)
	ctx := context.Background()

	// List seeded roles (admin, user).
	list, err := svc.ListRoles(ctx, connect.NewRequest(&zerxv1.ListRolesRequest{}))
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if len(list.Msg.GetRoles()) != 2 {
		t.Fatalf("seeded roles = %d, want 2", len(list.Msg.GetRoles()))
	}

	// Create a custom role.
	created, err := svc.CreateRole(ctx, connect.NewRequest(&zerxv1.CreateRoleRequest{
		Code: "editor", Name: "Editor",
	}))
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	// Duplicate code rejected.
	if _, err := svc.CreateRole(ctx, connect.NewRequest(&zerxv1.CreateRoleRequest{
		Code: "editor", Name: "Dup",
	})); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Errorf("duplicate role code = %v, want AlreadyExists", connect.CodeOf(err))
	}

	// Deleting a builtin role is rejected.
	roles := list.Msg.GetRoles()
	var adminID uint64
	for _, r := range roles {
		if r.GetCode() == "admin" {
			adminID = r.GetId()
		}
	}
	if _, err := svc.DeleteRole(ctx, connect.NewRequest(&zerxv1.DeleteRoleRequest{Id: adminID})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("delete builtin = %v, want FailedPrecondition", connect.CodeOf(err))
	}

	// Delete the custom role works.
	if _, err := svc.DeleteRole(ctx, connect.NewRequest(&zerxv1.DeleteRoleRequest{Id: created.Msg.GetId()})); err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}
}

func TestDeleteRoleInUseRejected(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)
	svc := NewRoleService(db, e)
	ctx := context.Background()

	created, err := svc.CreateRole(ctx, connect.NewRequest(&zerxv1.CreateRoleRequest{Code: "staff", Name: "Staff"}))
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	// Insert a user holding that role via user_roles table (no role column on users).
	u := model.User{Email: "s@x.com", Name: "S", PasswordHash: "h", Status: true}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := db.Create(&model.UserRole{UserID: u.ID, RoleCode: "staff"}).Error; err != nil {
		t.Fatalf("insert user_role: %v", err)
	}

	if _, err := svc.DeleteRole(ctx, connect.NewRequest(&zerxv1.DeleteRoleRequest{Id: created.Msg.GetId()})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("delete in-use role = %v, want FailedPrecondition", connect.CodeOf(err))
	}
}

// fakeRequest is a minimal connect.AnyRequest for the casbin interceptor test.
func enforceProc(t *testing.T, e *casbinlib.SyncedCachedEnforcer, role, proc string) bool {
	t.Helper()
	ok, err := e.Enforce(role, proc)
	if err != nil {
		t.Fatalf("Enforce: %v", err)
	}
	return ok
}

func TestSetRolePermissionsGrantsProcedure(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)
	svc := NewRoleService(db, e)
	ctx := context.Background()

	if _, err := svc.CreateRole(ctx, connect.NewRequest(&zerxv1.CreateRoleRequest{Code: "editor", Name: "Editor"})); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	proc := "/zerx.v1.UserService/CreateUser"
	if _, err := svc.SetRolePermissions(ctx, connect.NewRequest(&zerxv1.SetRolePermissionsRequest{
		RoleCode:   "editor",
		Procedures: []string{proc},
	})); err != nil {
		t.Fatalf("SetRolePermissions: %v", err)
	}

	if !enforceProc(t, e, "editor", proc) {
		t.Error("editor should be allowed CreateUser after grant (write authorization, plan decision A)")
	}
	if enforceProc(t, e, "editor", "/zerx.v1.UserService/DeleteUser") {
		t.Error("editor should NOT be allowed ungranted DeleteUser")
	}

	// Read it back via GetRolePermissions.
	got, err := svc.GetRolePermissions(ctx, connect.NewRequest(&zerxv1.GetRolePermissionsRequest{RoleCode: "editor"}))
	if err != nil {
		t.Fatalf("GetRolePermissions: %v", err)
	}
	if len(got.Msg.GetProcedures()) != 1 || got.Msg.GetProcedures()[0] != proc {
		t.Errorf("procedures = %v, want [%s]", got.Msg.GetProcedures(), proc)
	}
}

func TestGetUserMenusAdminAndUser(t *testing.T) {
	db := newTestDB(t)
	menuSvc := NewMenuService(db, nil)

	adminCtx := auth.WithClaims(context.Background(), &auth.Claims{UserID: 1, Roles: []string{"admin"}})
	adminMenus, err := menuSvc.GetUserMenus(adminCtx, connect.NewRequest(&zerxv1.GetUserMenusRequest{}))
	if err != nil {
		t.Fatalf("admin GetUserMenus: %v", err)
	}
	if len(adminMenus.Msg.GetMenus()) == 0 {
		t.Fatal("admin should see the full menu tree")
	}

	userCtx := auth.WithClaims(context.Background(), &auth.Claims{UserID: 2, Roles: []string{"user"}})
	userMenus, err := menuSvc.GetUserMenus(userCtx, connect.NewRequest(&zerxv1.GetUserMenusRequest{}))
	if err != nil {
		t.Fatalf("user GetUserMenus: %v", err)
	}
	// user role is seeded with dashboard + profile (both userVisible:true in seedMenuTree).
	count := countMenus(userMenus.Msg.GetMenus())
	if count < 1 {
		t.Errorf("user visible menus = %d, want at least 1", count)
	}
}

func countMenus(menus []*zerxv1.Menu) int {
	n := 0
	for _, m := range menus {
		n++
		n += countMenus(m.GetChildren())
	}
	return n
}

func TestGetUserMenusIncludesAncestorGroup(t *testing.T) {
	db := newTestDB(t)
	menuSvc := NewMenuService(db, nil)
	ctx := context.Background()

	// Find the "users" leaf menu (under the system group) and its parent.
	var usersMenu struct {
		ID       uint64
		ParentID uint64
	}
	if err := db.Raw("SELECT id, parent_id FROM menus WHERE path = ?", "/users").Scan(&usersMenu).Error; err != nil {
		t.Fatalf("query users menu: %v", err)
	}
	if usersMenu.ID == 0 || usersMenu.ParentID == 0 {
		t.Fatalf("users menu not found or has no parent: %+v", usersMenu)
	}

	// Grant only the leaf (not the parent group) to a custom role.
	if err := db.Exec("INSERT INTO roles (code, name, builtin) VALUES (?, ?, ?)", "leafonly", "Leaf", false).Error; err != nil {
		t.Fatalf("insert role: %v", err)
	}
	if err := db.Exec("INSERT INTO role_menus (role_code, menu_id) VALUES (?, ?)", "leafonly", usersMenu.ID).Error; err != nil {
		t.Fatalf("insert role_menu: %v", err)
	}

	leafCtx := auth.WithClaims(ctx, &auth.Claims{UserID: 3, Roles: []string{"leafonly"}})
	res, err := menuSvc.GetUserMenus(leafCtx, connect.NewRequest(&zerxv1.GetUserMenusRequest{}))
	if err != nil {
		t.Fatalf("GetUserMenus: %v", err)
	}

	// The parent group must be auto-included so the leaf is reachable.
	var foundParent, foundLeaf bool
	var walk func(menus []*zerxv1.Menu)
	walk = func(menus []*zerxv1.Menu) {
		for _, m := range menus {
			if m.GetId() == usersMenu.ParentID {
				foundParent = true
			}
			if m.GetId() == usersMenu.ID {
				foundLeaf = true
			}
			walk(m.GetChildren())
		}
	}
	walk(res.Msg.GetMenus())
	if !foundParent || !foundLeaf {
		t.Errorf("expected ancestor group + leaf present; parent=%v leaf=%v", foundParent, foundLeaf)
	}
}
