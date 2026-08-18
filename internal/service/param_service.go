package service

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/audit"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/param"
)

// SysParamService implements zerxv1connect.SysParamServiceHandler.
type SysParamService struct {
	db    *gorm.DB
	cache *param.Cache
}

var _ zerxv1connect.SysParamServiceHandler = (*SysParamService)(nil)

// NewSysParamService constructs the system parameter handler.
func NewSysParamService(db *gorm.DB, cache *param.Cache) *SysParamService {
	return &SysParamService{db: db, cache: cache}
}

func (s *SysParamService) ListParams(ctx context.Context, req *connect.Request[zerxv1.ListParamsRequest]) (*connect.Response[zerxv1.ListParamsResponse], error) {
	_, ps, offset := normalizePage(req.Msg.GetPage().GetPage(), req.Msg.GetPage().GetPageSize())
	like := "%" + req.Msg.GetKeyword() + "%"

	base := gorm.G[model.SysParam](s.db)
	if req.Msg.GetKeyword() != "" {
		filtered := base.Where("key LIKE ? OR name LIKE ?", like, like)
		total, err := filtered.Count(ctx, "id")
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		params, err := filtered.Order("id ASC").Limit(ps).Offset(offset).Find(ctx)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		return connect.NewResponse(&zerxv1.ListParamsResponse{Params: protoParams(params), Total: total}), nil
	}

	total, err := base.Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	params, err := gorm.G[model.SysParam](s.db).Order("id ASC").Limit(ps).Offset(offset).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.ListParamsResponse{Params: protoParams(params), Total: total}), nil
}

func protoParams(params []model.SysParam) []*zerxv1.SysParam {
	out := make([]*zerxv1.SysParam, 0, len(params))
	for i := range params {
		out = append(out, toProtoSysParam(params[i]))
	}

	return out
}

func (s *SysParamService) CreateParam(ctx context.Context, req *connect.Request[zerxv1.CreateParamRequest]) (*connect.Response[zerxv1.SysParam], error) {
	_, err := gorm.G[model.SysParam](s.db).Where("key = ?", req.Msg.GetKey()).First(ctx)
	switch {
	case err == nil:
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("param key already exists"))
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	p := model.SysParam{
		Key:         req.Msg.GetKey(),
		Name:        req.Msg.GetName(),
		Value:       req.Msg.GetValue(),
		Description: req.Msg.GetDescription(),
	}
	if err := gorm.G[model.SysParam](s.db).Create(ctx, &p); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.cache.Reload(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"id": p.ID, "key": p.Key, "name": p.Name}}))
	return connect.NewResponse(toProtoSysParam(p)), nil
}

func (s *SysParamService) UpdateParam(ctx context.Context, req *connect.Request[zerxv1.UpdateParamRequest]) (*connect.Response[zerxv1.SysParam], error) {
	id := req.Msg.GetId()
	old, err := gorm.G[model.SysParam](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("param not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	updates := map[string]any{
		"name":        req.Msg.GetName(),
		"value":       req.Msg.GetValue(),
		"description": req.Msg.GetDescription(),
	}
	if err := s.db.WithContext(ctx).Model(&model.SysParam{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	p, err := gorm.G[model.SysParam](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.cache.Reload(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"id": old.ID, "key": old.Key, "name": old.Name, "value": old.Value}, "after": map[string]any{"id": p.ID, "key": p.Key, "name": p.Name, "value": p.Value}}))
	return connect.NewResponse(toProtoSysParam(p)), nil
}

func (s *SysParamService) DeleteParam(ctx context.Context, req *connect.Request[zerxv1.DeleteParamRequest]) (*connect.Response[zerxv1.DeleteParamResponse], error) {
	id := req.Msg.GetId()
	old, err := gorm.G[model.SysParam](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("param not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	rows, err := gorm.G[model.SysParam](s.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("param not found"))
	}
	if err := s.cache.Reload(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"id": old.ID, "key": old.Key, "name": old.Name}}))
	return connect.NewResponse(&zerxv1.DeleteParamResponse{}), nil
}

func (s *SysParamService) GetParam(ctx context.Context, req *connect.Request[zerxv1.GetParamRequest]) (*connect.Response[zerxv1.SysParam], error) {
	p, err := gorm.G[model.SysParam](s.db).Where("key = ?", req.Msg.GetKey()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("param not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(toProtoSysParam(p)), nil
}
