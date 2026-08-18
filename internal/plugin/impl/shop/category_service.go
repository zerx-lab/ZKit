package shop

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/audit"
)

// CategoryService implements zerxv1connect.ShopCategoryServiceHandler.
type CategoryService struct {
	db *gorm.DB
}

var _ zerxv1connect.ShopCategoryServiceHandler = (*CategoryService)(nil)

// NewCategoryService constructs the category handler.
func NewCategoryService(db *gorm.DB) *CategoryService {
	return &CategoryService{db: db}
}

func (s *CategoryService) ListCategories(ctx context.Context, req *connect.Request[zerxv1.ListCategoriesRequest]) (*connect.Response[zerxv1.ListCategoriesResponse], error) {
	ps, offset := normalizePage(req.Msg.GetPage().GetPage(), req.Msg.GetPage().GetPageSize())
	total, err := gorm.G[Category](s.db).Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	rows, err := gorm.G[Category](s.db).Order("id ASC").Limit(ps).Offset(offset).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := make([]*zerxv1.ShopCategory, 0, len(rows))
	for i := range rows {
		out = append(out, toProtoCategory(rows[i]))
	}
	return connect.NewResponse(&zerxv1.ListCategoriesResponse{Categories: out, Total: total}), nil
}

func (s *CategoryService) CreateCategory(ctx context.Context, req *connect.Request[zerxv1.CreateCategoryRequest]) (*connect.Response[zerxv1.ShopCategory], error) {
	c := Category{Name: req.Msg.GetName()}
	if err := gorm.G[Category](s.db).Create(ctx, &c); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"id": c.ID, "name": c.Name}}))
	return connect.NewResponse(toProtoCategory(c)), nil
}

func (s *CategoryService) DeleteCategory(ctx context.Context, req *connect.Request[zerxv1.DeleteCategoryRequest]) (*connect.Response[zerxv1.DeleteCategoryResponse], error) {
	old, err := gorm.G[Category](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("category not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := gorm.G[Category](s.db).Where("id = ?", req.Msg.GetId()).Delete(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"id": old.ID, "name": old.Name}}))
	return connect.NewResponse(&zerxv1.DeleteCategoryResponse{}), nil
}

func toProtoCategory(c Category) *zerxv1.ShopCategory {
	return &zerxv1.ShopCategory{
		Id:        c.ID,
		Name:      c.Name,
		CreatedAt: c.CreatedAt.Format(time.RFC3339),
	}
}
