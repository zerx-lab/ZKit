package service

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/param"
	"github.com/zerx-lab/zkit/internal/storage"
)

func newSiteService(t *testing.T) *SiteSettingsService {
	t.Helper()
	db := newTestDB(t)
	cache := param.New(db)
	if err := cache.Load(context.Background()); err != nil {
		t.Fatalf("load params: %v", err)
	}
	cfg := config.StorageConfig{Driver: "local", LocalDir: t.TempDir(), LocalBaseURL: "/uploads", SignedURLTTL: time.Hour}
	store, err := storage.New(cfg)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	return NewSiteSettingsService(db, cache, media.New(store, cfg, []byte("test-sign-key")))
}

func TestGetSiteSettingsRegisterDefaults(t *testing.T) {
	svc := newSiteService(t)
	res, err := svc.GetSiteSettings(context.Background(), connect.NewRequest(&zerxv1.GetSiteSettingsRequest{}))
	if err != nil {
		t.Fatalf("GetSiteSettings: %v", err)
	}
	if !res.Msg.GetRegisterEnabled() {
		t.Fatal("register_enabled default want true")
	}
	if !res.Msg.GetRegisterOpen() {
		t.Fatal("register_open want true by default")
	}
	if res.Msg.GetRegisterDefaultRole() != model.RoleUser {
		t.Fatalf("default role = %q, want user", res.Msg.GetRegisterDefaultRole())
	}
}

func TestUpdateSiteSettingsRejectsAdminDefaultRole(t *testing.T) {
	svc := newSiteService(t)
	_, err := svc.UpdateSiteSettings(context.Background(), connect.NewRequest(&zerxv1.UpdateSiteSettingsRequest{
		RegisterEnabled:     true,
		RegisterDefaultRole: model.RoleAdmin,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("update admin default = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestUpdateSiteSettingsRejectsUnknownDefaultRole(t *testing.T) {
	svc := newSiteService(t)
	_, err := svc.UpdateSiteSettings(context.Background(), connect.NewRequest(&zerxv1.UpdateSiteSettingsRequest{
		RegisterDefaultRole: "missing",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("update missing role = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestGetSiteSettingsRegisterClosedWhenDisabled(t *testing.T) {
	db := newTestDB(t)
	cache := param.New(db)
	_ = cache.Load(context.Background())
	cfg := config.StorageConfig{Driver: "local", LocalDir: t.TempDir(), LocalBaseURL: "/uploads", SignedURLTTL: time.Hour}
	store, _ := storage.New(cfg)
	svc := NewSiteSettingsService(db, cache, media.New(store, cfg, []byte("test-sign-key")))
	seedUser(t, db, "a@b.com", "password1", model.RoleAdmin)
	if err := cache.Set(context.Background(), siteRegisterEnabledKey, "false"); err != nil {
		t.Fatalf("disable register: %v", err)
	}

	res, err := svc.GetSiteSettings(context.Background(), connect.NewRequest(&zerxv1.GetSiteSettingsRequest{}))
	if err != nil {
		t.Fatalf("GetSiteSettings: %v", err)
	}
	if res.Msg.GetRegisterEnabled() || res.Msg.GetRegisterOpen() {
		t.Fatal("register want closed after explicit disable")
	}
}
