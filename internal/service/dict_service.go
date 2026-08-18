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
)

// DictService implements zerxv1connect.DictServiceHandler.
type DictService struct {
	db *gorm.DB
}

var _ zerxv1connect.DictServiceHandler = (*DictService)(nil)

// NewDictService constructs the dictionary handler.
func NewDictService(db *gorm.DB) *DictService {
	return &DictService{db: db}
}

func (s *DictService) ListDicts(ctx context.Context, req *connect.Request[zerxv1.ListDictsRequest]) (*connect.Response[zerxv1.ListDictsResponse], error) {
	_, ps, offset := normalizePage(req.Msg.GetPage().GetPage(), req.Msg.GetPage().GetPageSize())
	like := "%" + req.Msg.GetKeyword() + "%"
	hasKw := req.Msg.GetKeyword() != ""

	cnt := gorm.G[model.Dictionary](s.db)
	if hasKw {
		cntF := cnt.Where("type LIKE ? OR name LIKE ?", like, like)
		total, err := cntF.Count(ctx, "id")
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		dicts, err := cntF.Order("id ASC").Limit(ps).Offset(offset).Find(ctx)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		return connect.NewResponse(&zerxv1.ListDictsResponse{Dicts: protoDicts(dicts), Total: total}), nil
	}

	total, err := cnt.Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	dicts, err := gorm.G[model.Dictionary](s.db).Order("id ASC").Limit(ps).Offset(offset).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.ListDictsResponse{Dicts: protoDicts(dicts), Total: total}), nil
}

func protoDicts(dicts []model.Dictionary) []*zerxv1.Dict {
	out := make([]*zerxv1.Dict, 0, len(dicts))
	for i := range dicts {
		out = append(out, toProtoDict(dicts[i]))
	}

	return out
}

func (s *DictService) CreateDict(ctx context.Context, req *connect.Request[zerxv1.CreateDictRequest]) (*connect.Response[zerxv1.Dict], error) {
	_, err := gorm.G[model.Dictionary](s.db).Where("type = ?", req.Msg.GetType()).First(ctx)
	switch {
	case err == nil:
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("dictionary type already exists"))
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	d := model.Dictionary{
		Type:        req.Msg.GetType(),
		Name:        req.Msg.GetName(),
		Description: req.Msg.GetDescription(),
		Status:      req.Msg.GetStatus(),
	}
	if err := gorm.G[model.Dictionary](s.db).Create(ctx, &d); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"id": d.ID, "type": d.Type, "name": d.Name, "status": d.Status}}))
	return connect.NewResponse(toProtoDict(d)), nil
}

func (s *DictService) UpdateDict(ctx context.Context, req *connect.Request[zerxv1.UpdateDictRequest]) (*connect.Response[zerxv1.Dict], error) {
	id := req.Msg.GetId()
	old, err := gorm.G[model.Dictionary](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("dictionary not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	updates := map[string]any{
		"name":        req.Msg.GetName(),
		"description": req.Msg.GetDescription(),
		"status":      req.Msg.GetStatus(),
	}
	if err := s.db.WithContext(ctx).Model(&model.Dictionary{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	d, err := gorm.G[model.Dictionary](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"id": old.ID, "type": old.Type, "name": old.Name, "status": old.Status}, "after": map[string]any{"id": d.ID, "type": d.Type, "name": d.Name, "status": d.Status}}))
	return connect.NewResponse(toProtoDict(d)), nil
}

func (s *DictService) DeleteDict(ctx context.Context, req *connect.Request[zerxv1.DeleteDictRequest]) (*connect.Response[zerxv1.DeleteDictResponse], error) {
	id := req.Msg.GetId()
	old, err := gorm.G[model.Dictionary](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("dictionary not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("dict_id = ?", id).Delete(&model.DictionaryItem{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ?", id).Delete(&model.Dictionary{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return nil
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("dictionary not found"))
	}
	if txErr != nil {
		return nil, connect.NewError(connect.CodeInternal, txErr)
	}

	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"id": old.ID, "type": old.Type, "name": old.Name}}))
	return connect.NewResponse(&zerxv1.DeleteDictResponse{}), nil
}

func (s *DictService) ListDictItems(ctx context.Context, req *connect.Request[zerxv1.ListDictItemsRequest]) (*connect.Response[zerxv1.ListDictItemsResponse], error) {
	items, err := gorm.G[model.DictionaryItem](s.db).Where("dict_id = ?", req.Msg.GetDictId()).Order("sort ASC, id ASC").Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	out := make([]*zerxv1.DictItem, 0, len(items))
	for i := range items {
		out = append(out, toProtoDictItem(items[i]))
	}

	return connect.NewResponse(&zerxv1.ListDictItemsResponse{Items: out}), nil
}

func (s *DictService) CreateDictItem(ctx context.Context, req *connect.Request[zerxv1.CreateDictItemRequest]) (*connect.Response[zerxv1.DictItem], error) {
	item := model.DictionaryItem{
		DictID: req.Msg.GetDictId(),
		Label:  req.Msg.GetLabel(),
		Value:  req.Msg.GetValue(),
		Sort:   int(req.Msg.GetSort()),
		Status: req.Msg.GetStatus(),
	}
	if err := gorm.G[model.DictionaryItem](s.db).Create(ctx, &item); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"id": item.ID, "dict_id": item.DictID, "label": item.Label, "value": item.Value}}))
	return connect.NewResponse(toProtoDictItem(item)), nil
}

func (s *DictService) UpdateDictItem(ctx context.Context, req *connect.Request[zerxv1.UpdateDictItemRequest]) (*connect.Response[zerxv1.DictItem], error) {
	id := req.Msg.GetId()
	old, err := gorm.G[model.DictionaryItem](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("item not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	updates := map[string]any{
		"label":  req.Msg.GetLabel(),
		"value":  req.Msg.GetValue(),
		"sort":   int(req.Msg.GetSort()),
		"status": req.Msg.GetStatus(),
	}
	if err := s.db.WithContext(ctx).Model(&model.DictionaryItem{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	item, err := gorm.G[model.DictionaryItem](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"id": old.ID, "dict_id": old.DictID, "label": old.Label, "value": old.Value}, "after": map[string]any{"id": item.ID, "dict_id": item.DictID, "label": item.Label, "value": item.Value}}))
	return connect.NewResponse(toProtoDictItem(item)), nil
}

func (s *DictService) DeleteDictItem(ctx context.Context, req *connect.Request[zerxv1.DeleteDictItemRequest]) (*connect.Response[zerxv1.DeleteDictItemResponse], error) {
	id := req.Msg.GetId()
	old, err := gorm.G[model.DictionaryItem](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("item not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	rows, err := gorm.G[model.DictionaryItem](s.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("item not found"))
	}
	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"id": old.ID, "dict_id": old.DictID, "label": old.Label, "value": old.Value}}))
	return connect.NewResponse(&zerxv1.DeleteDictItemResponse{}), nil
}

func (s *DictService) GetDictByType(ctx context.Context, req *connect.Request[zerxv1.GetDictByTypeRequest]) (*connect.Response[zerxv1.GetDictByTypeResponse], error) {
	dict, err := gorm.G[model.Dictionary](s.db).Where("type = ?", req.Msg.GetType()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return connect.NewResponse(&zerxv1.GetDictByTypeResponse{}), nil
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	items, err := gorm.G[model.DictionaryItem](s.db).Where("dict_id = ? AND status = ?", dict.ID, true).Order("sort ASC, id ASC").Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	out := make([]*zerxv1.DictItem, 0, len(items))
	for i := range items {
		out = append(out, toProtoDictItem(items[i]))
	}

	return connect.NewResponse(&zerxv1.GetDictByTypeResponse{Items: out}), nil
}
