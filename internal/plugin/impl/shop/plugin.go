// Package shop is the example ZKit plugin: a product CRUD module that
// exercises the full plugin contract (proto service, migration, namespaced
// table, seed menu, dynamic frontend page, public page, and an optional job).
package shop

import (
	"context"
	"errors"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/plugin"
)

// pluginName is the plugin identifier (lower snake_case).
const pluginName = "shop"

// Plugin implements plugin.Plugin for the shop module.
type Plugin struct{}

var _ plugin.Plugin = (*Plugin)(nil)

// New constructs the shop plugin.
func New() *Plugin { return &Plugin{} }

// Name returns the plugin identifier.
func (*Plugin) Name() string { return pluginName }

// Services returns the connectRPC services this plugin owns.
func (*Plugin) Services() []string {
	return []string{"zerx.v1.ShopProductService", "zerx.v1.ShopCategoryService"}
}

// Migrations returns the plugin's schema migrations (IDs prefixed plg_shop_).
func (*Plugin) Migrations() []*gormigrate.Migration {
	return []*gormigrate.Migration{
		{
			ID: "plg_shop_0001_products",
			Migrate: func(tx *gorm.DB) error {
				return tx.AutoMigrate(&Product{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable(&Product{})
			},
		},
		{
			ID: "plg_shop_0002_categories",
			Migrate: func(tx *gorm.DB) error {
				return tx.AutoMigrate(&Category{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable(&Category{})
			},
		},
	}
}

// TableNames returns the plugin's owned tables (prefixed plg_shop_).
func (*Plugin) TableNames() []string {
	return []string{"plg_shop_products", "plg_shop_categories"}
}

// SeedMenus returns the plugin's navigation subtree: a "shop" group heading
// (Path=="") with two leaf sub-pages, demonstrating a grouped plugin layout.
func (*Plugin) SeedMenus() []plugin.MenuNode {
	return []plugin.MenuNode{
		{
			Name:  "plg_shop",
			Path:  "", // group heading
			Title: "plg.shop.group",
			Icon:  "ShoppingBagIcon",
			Sort:  50,
			Children: []plugin.MenuNode{
				{
					Name:      "plg_shop_products",
					Path:      "/p/shop/products",
					Component: "shop/Products",
					Title:     "plg.shop.products",
					Icon:      "ShoppingBagIcon",
					Sort:      1,
					Buttons: []plugin.MenuButton{
						{Code: "plg_shop_product:create", Name: "商品新增"},
						{Code: "plg_shop_product:update", Name: "商品编辑"},
						{Code: "plg_shop_product:delete", Name: "商品删除"},
					},
				},
				{
					Name:      "plg_shop_categories",
					Path:      "/p/shop/categories",
					Component: "shop/Categories",
					Title:     "plg.shop.categories",
					Icon:      "ListTreeIcon",
					Sort:      2,
					Buttons: []plugin.MenuButton{
						{Code: "plg_shop_category:create", Name: "分类新增"},
						{Code: "plg_shop_category:delete", Name: "分类删除"},
					},
				},
			},
		},
	}
}

// PublicProcedures returns no unauthenticated procedures.
func (*Plugin) PublicProcedures() []string { return nil }

// SelfServeProcedures returns no self-serve procedures (all CRUD goes through
// Casbin; admin bypasses, other roles require an explicit grant).
func (*Plugin) SelfServeProcedures() []string { return nil }

// RegisterHandlers mounts both shop services via the shared reg + opts.
func (*Plugin) RegisterHandlers(reg plugin.RegFunc, deps plugin.Deps) {
	reg(zerxv1connect.NewShopProductServiceHandler(NewProductService(deps.DB), deps.Opts))
	reg(zerxv1connect.NewShopCategoryServiceHandler(NewCategoryService(deps.DB), deps.Opts))
}

// PublicPages exposes one anonymous front-end page (a storefront landing) to
// demonstrate public-facing plugin routes served under /pub without login.
func (*Plugin) PublicPages() []plugin.PublicPage {
	return []plugin.PublicPage{
		{Path: "/pub/shop", Component: "shop/Landing", Title: "plg.shop.landing"},
	}
}

// JobHandlers exposes a demo job that exercises the scheduler panic-isolation
// path; it is registered but only runs if an admin schedules/RunNow's it.
func (*Plugin) JobHandlers() map[string]plugin.JobHandler {
	return map[string]plugin.JobHandler{
		"shop_demo_panic": {
			Description: "示例插件:故意 panic 的任务(验证调度器隔离)",
			Run: func(_ context.Context) error {
				panic(errors.New("shop demo panic"))
			},
		},
	}
}
