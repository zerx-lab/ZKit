package shop

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/audit"
)

// ProductService implements zerxv1connect.ShopProductServiceHandler.
type ProductService struct {
	db *gorm.DB
}

var _ zerxv1connect.ShopProductServiceHandler = (*ProductService)(nil)

// NewProductService constructs the product handler.
func NewProductService(db *gorm.DB) *ProductService {
	return &ProductService{db: db}
}

func (s *ProductService) ListProducts(ctx context.Context, req *connect.Request[zerxv1.ListProductsRequest]) (*connect.Response[zerxv1.ListProductsResponse], error) {
	ps, offset := normalizePage(req.Msg.GetPage().GetPage(), req.Msg.GetPage().GetPageSize())
	if kw := req.Msg.GetKeyword(); kw != "" {
		q := gorm.G[Product](s.db).Where("name LIKE ?", "%"+kw+"%")
		total, err := q.Count(ctx, "id")
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		rows, err := q.Order("id ASC").Limit(ps).Offset(offset).Find(ctx)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		return connect.NewResponse(&zerxv1.ListProductsResponse{Products: protoProducts(rows), Total: total}), nil
	}
	total, err := gorm.G[Product](s.db).Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	rows, err := gorm.G[Product](s.db).Order("id ASC").Limit(ps).Offset(offset).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&zerxv1.ListProductsResponse{Products: protoProducts(rows), Total: total}), nil
}

func (s *ProductService) GetProduct(ctx context.Context, req *connect.Request[zerxv1.GetProductRequest]) (*connect.Response[zerxv1.ShopProduct], error) {
	p, err := gorm.G[Product](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("product not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(toProtoProduct(p)), nil
}

func (s *ProductService) CreateProduct(ctx context.Context, req *connect.Request[zerxv1.CreateProductRequest]) (*connect.Response[zerxv1.ShopProduct], error) {
	p := Product{Name: req.Msg.GetName(), Price: req.Msg.GetPrice(), Description: req.Msg.GetDescription()}
	if err := gorm.G[Product](s.db).Create(ctx, &p); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"id": p.ID, "name": p.Name, "price": p.Price}}))
	return connect.NewResponse(toProtoProduct(p)), nil
}

func (s *ProductService) UpdateProduct(ctx context.Context, req *connect.Request[zerxv1.UpdateProductRequest]) (*connect.Response[zerxv1.ShopProduct], error) {
	old, err := gorm.G[Product](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("product not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.db.WithContext(ctx).Model(&Product{}).Where("id = ?", req.Msg.GetId()).Updates(map[string]any{
		"name":        req.Msg.GetName(),
		"price":       req.Msg.GetPrice(),
		"description": req.Msg.GetDescription(),
	}).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	p, err := gorm.G[Product](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{
		"before": map[string]any{"id": old.ID, "name": old.Name, "price": old.Price},
		"after":  map[string]any{"id": p.ID, "name": p.Name, "price": p.Price},
	}))
	return connect.NewResponse(toProtoProduct(p)), nil
}

func (s *ProductService) DeleteProduct(ctx context.Context, req *connect.Request[zerxv1.DeleteProductRequest]) (*connect.Response[zerxv1.DeleteProductResponse], error) {
	old, err := gorm.G[Product](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("product not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := gorm.G[Product](s.db).Where("id = ?", req.Msg.GetId()).Delete(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"id": old.ID, "name": old.Name}}))
	return connect.NewResponse(&zerxv1.DeleteProductResponse{}), nil
}

func protoProducts(rows []Product) []*zerxv1.ShopProduct {
	out := make([]*zerxv1.ShopProduct, 0, len(rows))
	for i := range rows {
		out = append(out, toProtoProduct(rows[i]))
	}
	return out
}

func toProtoProduct(p Product) *zerxv1.ShopProduct {
	return &zerxv1.ShopProduct{
		Id:          p.ID,
		Name:        p.Name,
		Price:       p.Price,
		Description: p.Description,
		CreatedAt:   p.CreatedAt.Format(time.RFC3339),
	}
}

// normalizePage clamps page/pageSize and returns the SQL limit + offset.
func normalizePage(page, pageSize int32) (ps, offset int) {
	p := int(page)
	if p < 1 {
		p = 1
	}
	ps = int(pageSize)
	if ps < 1 || ps > 200 {
		ps = 20
	}
	return ps, (p - 1) * ps
}

func auditJSON(v map[string]any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
