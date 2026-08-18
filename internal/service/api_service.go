package service

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	casbinpkg "github.com/casbin/casbin/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/apispec"
	"github.com/zerx-lab/zkit/internal/casbin"
	"github.com/zerx-lab/zkit/internal/model"
)

// ApiService implements zerxv1connect.ApiServiceHandler.
type ApiService struct {
	db       *gorm.DB
	enforcer *casbinpkg.SyncedCachedEnforcer
}

var _ zerxv1connect.ApiServiceHandler = (*ApiService)(nil)

// NewApiService constructs the API catalog handler.
func NewApiService(db *gorm.DB, enforcer *casbinpkg.SyncedCachedEnforcer) *ApiService {
	return &ApiService{db: db, enforcer: enforcer}
}

func (s *ApiService) ListApis(ctx context.Context, _ *connect.Request[zerxv1.ListApisRequest]) (*connect.Response[zerxv1.ListApisResponse], error) {
	apis, err := gorm.G[model.API](s.db).Order("`group` ASC, procedure ASC").Find(ctx)
	if err != nil {
		// Fallback for engines where `group` quoting differs.
		apis, err = gorm.G[model.API](s.db).Order("procedure ASC").Find(ctx)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	out := make([]*zerxv1.Api, 0, len(apis))
	for i := range apis {
		out = append(out, toProtoAPI(apis[i]))
	}

	return connect.NewResponse(&zerxv1.ListApisResponse{Apis: out}), nil
}

func (s *ApiService) SyncApis(ctx context.Context, _ *connect.Request[zerxv1.SyncApisRequest]) (*connect.Response[zerxv1.SyncApisResponse], error) {
	procs := apispec.Procedures()
	live := make(map[string]apispec.Proc, len(procs))
	for _, p := range procs {
		live[p.Procedure] = p
	}

	existing, err := gorm.G[model.API](s.db).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	have := make(map[string]bool, len(existing))
	for i := range existing {
		have[existing[i].Procedure] = true
	}

	var added, removed int32

	// Insert new procedures (skip existing to preserve admin edits).
	newRows := make([]model.API, 0)
	for _, p := range procs {
		if !have[p.Procedure] {
			newRows = append(newRows, model.API{
				Procedure: p.Procedure,
				Service:   p.Service,
				Method:    p.Method,
				Group:     shortServiceName(p.Service),
			})
			added++
		}
	}
	if len(newRows) > 0 {
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&newRows).Error; err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	// Physically delete stale rows (the unique procedure index forbids soft
	// delete + re-insert) and clear their casbin policies.
	for i := range existing {
		if _, ok := live[existing[i].Procedure]; !ok {
			if err := s.db.WithContext(ctx).Unscoped().Where("id = ?", existing[i].ID).Delete(&model.API{}).Error; err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			if err := casbin.RemoveProcedure(s.enforcer, existing[i].Procedure); err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			removed++
		}
	}

	return connect.NewResponse(&zerxv1.SyncApisResponse{Added: added, Removed: removed}), nil
}

func (s *ApiService) UpdateApi(ctx context.Context, req *connect.Request[zerxv1.UpdateApiRequest]) (*connect.Response[zerxv1.Api], error) {
	id := req.Msg.GetId()
	if _, err := gorm.G[model.API](s.db).Where("id = ?", id).First(ctx); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("api not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	updates := map[string]any{
		"description": req.Msg.GetDescription(),
		"group":       req.Msg.GetGroup(),
	}
	if err := s.db.WithContext(ctx).Model(&model.API{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	a, err := gorm.G[model.API](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(toProtoAPI(a)), nil
}

func (s *ApiService) DeleteApi(ctx context.Context, req *connect.Request[zerxv1.DeleteApiRequest]) (*connect.Response[zerxv1.DeleteApiResponse], error) {
	a, err := gorm.G[model.API](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("api not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if err := s.db.WithContext(ctx).Unscoped().Where("id = ?", a.ID).Delete(&model.API{}).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := casbin.RemoveProcedure(s.enforcer, a.Procedure); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.DeleteApiResponse{}), nil
}

// shortServiceName returns the trailing dotted segment of a service name.
func shortServiceName(full string) string {
	for i := len(full) - 1; i >= 0; i-- {
		if full[i] == '.' {
			return full[i+1:]
		}
	}

	return full
}
