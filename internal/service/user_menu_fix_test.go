package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/storage"
)

func newUserService(t *testing.T, db *gorm.DB) *UserService {
	t.Helper()
	policy := auth.NewPolicy(config.PasswordPolicyConfig{MinLength: 8, HistoryCount: 3})
	cfg := config.StorageConfig{Driver: "local", LocalDir: t.TempDir(), LocalBaseURL: "/uploads", SignedURLTTL: time.Hour}
	store, err := storage.New(cfg)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	return NewUserService(db, policy, media.New(store, cfg, []byte("test-sign-key")))
}

func countWhere(t *testing.T, db *gorm.DB, m any, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Model(m).Where(query, args...).Count(&n).Error; err != nil {
		t.Fatalf("count %T: %v", m, err)
	}
	return n
}

func TestDeleteUserTombstonesAndCascades(t *testing.T) {
	db := newTestDB(t)
	svc := newUserService(t, db)
	ctx := context.Background()

	const email = "gone@example.com"
	hash, err := auth.Hash("password1")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := model.User{Email: email, Name: "Gone", PasswordHash: hash, Status: true}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	exp := time.Now().Add(time.Hour)
	for _, row := range []any{
		&model.UserRole{UserID: u.ID, RoleCode: model.RoleUser},
		&model.UserSession{ID: "sess-1", UserID: u.ID, ExpiresAt: exp},
		&model.UserTOTP{UserID: u.ID, Secret: "secret", Enabled: true},
		&model.TOTPRecoveryCode{UserID: u.ID, CodeHash: "h"},
		&model.PasswordHistory{UserID: u.ID, PasswordHash: hash},
		&model.PasswordResetToken{TokenHash: "tok", UserID: u.ID, ExpiresAt: exp},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create %T: %v", row, err)
		}
	}

	if _, err := svc.DeleteUser(ctx, connect.NewRequest(&zerxv1.DeleteUserRequest{Id: u.ID})); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	// Soft-deleted row is gone from scoped queries but recoverable via Unscoped,
	// with the email tombstoned so the unique index is released.
	if n := countWhere(t, db, &model.User{}, "id = ?", u.ID); n != 0 {
		t.Fatalf("scoped user rows = %d, want 0", n)
	}
	var old model.User
	if err := db.Unscoped().Where("id = ?", u.ID).First(&old).Error; err != nil {
		t.Fatalf("unscoped reload: %v", err)
	}
	if !old.DeletedAt.Valid {
		t.Fatal("expected deleted_at to be set")
	}
	if !model.IsTombstonedEmail(old.Email) {
		t.Fatalf("email = %q, want tombstoned", old.Email)
	}

	for _, m := range []any{
		&model.UserRole{}, &model.UserSession{}, &model.UserTOTP{},
		&model.TOTPRecoveryCode{}, &model.PasswordHistory{}, &model.PasswordResetToken{},
	} {
		if n := countWhere(t, db, m, "user_id = ?", u.ID); n != 0 {
			t.Errorf("%T rows for user %d = %d, want 0", m, u.ID, n)
		}
	}

	// Same email can be registered again.
	created, err := svc.CreateUser(ctx, connect.NewRequest(&zerxv1.CreateUserRequest{
		Email: email, Name: "Again", Password: "password1", Roles: []string{model.RoleUser},
	}))
	if err != nil {
		t.Fatalf("CreateUser with reused email: %v", err)
	}
	if created.Msg.GetId() == u.ID {
		t.Fatal("expected a new user id")
	}

	// Deleting again reports not found.
	_, err = svc.DeleteUser(ctx, connect.NewRequest(&zerxv1.DeleteUserRequest{Id: u.ID}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("second delete code = %v, want NotFound", connect.CodeOf(err))
	}
}

func TestListUsersKeywordPaginates(t *testing.T) {
	db := newTestDB(t)
	svc := newUserService(t, db)
	ctx := context.Background()

	for i := range 3 {
		seedUser(t, db, fmt.Sprintf("kw%d@example.com", i), "password1", model.RoleUser)
	}
	seedUser(t, db, "other@example.com", "password1", model.RoleUser)

	res, err := svc.ListUsers(ctx, connect.NewRequest(&zerxv1.ListUsersRequest{
		Keyword: "kw",
		Page:    &zerxv1.PageRequest{Page: 1, PageSize: 1},
	}))
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if got := len(res.Msg.GetUsers()); got != 1 {
		t.Fatalf("users on page = %d, want 1", got)
	}
	if res.Msg.GetTotal() != 3 {
		t.Fatalf("total = %d, want 3", res.Msg.GetTotal())
	}

	// Second page yields the next match, not the same one.
	res2, err := svc.ListUsers(ctx, connect.NewRequest(&zerxv1.ListUsersRequest{
		Keyword: "kw",
		Page:    &zerxv1.PageRequest{Page: 2, PageSize: 1},
	}))
	if err != nil {
		t.Fatalf("ListUsers page 2: %v", err)
	}
	if len(res2.Msg.GetUsers()) != 1 || res2.Msg.GetUsers()[0].GetId() == res.Msg.GetUsers()[0].GetId() {
		t.Fatalf("page 2 should return a different single user")
	}
}

func TestDeleteMenuRejectsWhenChildrenExist(t *testing.T) {
	db := newTestDB(t)
	svc := NewMenuService(db, nil)
	ctx := context.Background()

	parent := model.Menu{Name: "fixparent", Path: "/fixparent", Title: "Parent"}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child := model.Menu{ParentID: parent.ID, Name: "fixchild", Path: "/fixparent/child", Title: "Child"}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}

	_, err := svc.DeleteMenu(ctx, connect.NewRequest(&zerxv1.DeleteMenuRequest{Id: parent.ID}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("delete parent code = %v, want FailedPrecondition", connect.CodeOf(err))
	}
	if n := countWhere(t, db, &model.Menu{}, "id = ?", parent.ID); n != 1 {
		t.Fatalf("parent should still exist, rows = %d", n)
	}

	// Leaf deletes fine; afterwards the parent can be deleted too.
	if _, err := svc.DeleteMenu(ctx, connect.NewRequest(&zerxv1.DeleteMenuRequest{Id: child.ID})); err != nil {
		t.Fatalf("delete child: %v", err)
	}
	if _, err := svc.DeleteMenu(ctx, connect.NewRequest(&zerxv1.DeleteMenuRequest{Id: parent.ID})); err != nil {
		t.Fatalf("delete parent after child removed: %v", err)
	}
	if n := countWhere(t, db, &model.Menu{}, "id IN ?", []uint64{parent.ID, child.ID}); n != 0 {
		t.Fatalf("menus remaining = %d, want 0", n)
	}
}
