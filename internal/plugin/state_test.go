package plugin

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/model"
)

type statePlugin struct {
	fakePlugin
	jobs map[string]JobHandler
}

func (s statePlugin) JobHandlers() map[string]JobHandler { return s.jobs }

func newStateDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.PluginState{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestStateDefaultEnabledAndToggle(t *testing.T) {
	reset()
	t.Cleanup(reset)
	Register(statePlugin{
		fakePlugin: fakePlugin{name: "shop", services: []string{"zerx.v1.ShopProductService"}},
		jobs:       map[string]JobHandler{"shop_cleanup": {Run: func(context.Context) error { return nil }}},
	})

	db := newStateDB(t)
	st, err := NewState(db)
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}

	// Default-on: no row means enabled.
	if !st.IsEnabled("shop") {
		t.Fatal("expected shop enabled by default")
	}
	if !st.IsProcedureEnabled("/zerx.v1.ShopProductService/CreateProduct") {
		t.Fatal("expected plugin procedure enabled by default")
	}
	if !st.IsProcedureEnabled("/zerx.v1.UserService/DeleteUser") {
		t.Fatal("core procedure must always be enabled here")
	}
	if !st.IsJobHandlerEnabled("shop_cleanup") {
		t.Fatal("expected plugin job enabled by default")
	}
	if !st.IsJobHandlerEnabled("log_cleanup") {
		t.Fatal("core job must always be enabled here")
	}

	// Disable -> all three gates flip; persists across a Reload.
	if err := st.SetEnabled(context.Background(), "shop", false); err != nil {
		t.Fatalf("SetEnabled false: %v", err)
	}
	if st.IsEnabled("shop") || st.IsProcedureEnabled("/zerx.v1.ShopProductService/CreateProduct") || st.IsJobHandlerEnabled("shop_cleanup") {
		t.Fatal("expected shop fully gated after disable")
	}
	if err := st.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if st.IsEnabled("shop") {
		t.Fatal("disabled state must persist across reload")
	}

	// Re-enable.
	if err := st.SetEnabled(context.Background(), "shop", true); err != nil {
		t.Fatalf("SetEnabled true: %v", err)
	}
	if !st.IsEnabled("shop") {
		t.Fatal("expected shop enabled after re-enable")
	}
}

func TestEnabledPublicPagesOmitsDisabled(t *testing.T) {
	reset()
	t.Cleanup(reset)
	Register(statePlugin{fakePlugin: fakePlugin{
		name:        "shop",
		publicPages: []PublicPage{{Path: "/pub/shop", Component: "shop/Landing"}},
	}})

	db := newStateDB(t)
	st, err := NewState(db)
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}

	if got := st.EnabledPublicPages(); len(got) != 1 || got[0].Path != "/pub/shop" {
		t.Fatalf("expected 1 enabled public page, got %+v", got)
	}
	if err := st.SetEnabled(context.Background(), "shop", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if got := st.EnabledPublicPages(); len(got) != 0 {
		t.Fatalf("expected no public pages when disabled, got %+v", got)
	}
}

func TestPluginNameOfMenu(t *testing.T) {
	reset()
	t.Cleanup(reset)
	Register(fakePlugin{name: "shop"})

	cases := map[string]string{
		"plg_shop":            "shop",
		"plg_shop_products":   "shop",
		"plg_shopping_widget": "", // prefix overlap must not match
		"users":               "",
		"plg_other":           "",
	}
	for in, want := range cases {
		if got := PluginNameOfMenu(in); got != want {
			t.Errorf("PluginNameOfMenu(%q) = %q, want %q", in, got, want)
		}
	}
}
