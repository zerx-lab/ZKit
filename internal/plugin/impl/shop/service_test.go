package shop

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
)

func newShopDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Apply the plugin's own migrations (mirrors startup).
	for _, m := range (&Plugin{}).Migrations() {
		if err := m.Migrate(db); err != nil {
			t.Fatalf("migrate %s: %v", m.ID, err)
		}
	}
	return db
}

func TestProductCRUD(t *testing.T) {
	db := newShopDB(t)
	if !db.Migrator().HasTable("plg_shop_products") {
		t.Fatal("plg_shop_products table not created by migration")
	}
	svc := NewProductService(db)
	ctx := context.Background()

	created, err := svc.CreateProduct(ctx, connect.NewRequest(&zerxv1.CreateProductRequest{
		Name: "Widget", Price: 1999, Description: "a widget",
	}))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	id := created.Msg.GetId()
	if id == 0 {
		t.Fatal("CreateProduct returned zero id")
	}

	got, err := svc.GetProduct(ctx, connect.NewRequest(&zerxv1.GetProductRequest{Id: id}))
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if got.Msg.GetName() != "Widget" || got.Msg.GetPrice() != 1999 {
		t.Fatalf("GetProduct = %+v, want Widget/1999", got.Msg)
	}

	if _, err := svc.UpdateProduct(ctx, connect.NewRequest(&zerxv1.UpdateProductRequest{
		Id: id, Name: "Gadget", Price: 2500, Description: "updated",
	})); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}

	list, err := svc.ListProducts(ctx, connect.NewRequest(&zerxv1.ListProductsRequest{
		Keyword: "Gadget",
	}))
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if list.Msg.GetTotal() != 1 || len(list.Msg.GetProducts()) != 1 {
		t.Fatalf("ListProducts total = %d, want 1", list.Msg.GetTotal())
	}
	if list.Msg.GetProducts()[0].GetName() != "Gadget" {
		t.Fatalf("updated name = %q, want Gadget", list.Msg.GetProducts()[0].GetName())
	}

	if _, err := svc.DeleteProduct(ctx, connect.NewRequest(&zerxv1.DeleteProductRequest{Id: id})); err != nil {
		t.Fatalf("DeleteProduct: %v", err)
	}
	if _, err := svc.GetProduct(ctx, connect.NewRequest(&zerxv1.GetProductRequest{Id: id})); err == nil {
		t.Fatal("GetProduct after delete: expected not-found error")
	}
}

func TestGetProductNotFound(t *testing.T) {
	db := newShopDB(t)
	svc := NewProductService(db)
	_, err := svc.GetProduct(context.Background(), connect.NewRequest(&zerxv1.GetProductRequest{Id: 999}))
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("error code = %v, want NotFound", connect.CodeOf(err))
	}
}
