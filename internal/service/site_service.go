package service

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/audit"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/param"
)

// Fixed parameter keys backing the site settings.
const (
	siteNameKey                = "site.name"
	siteLogoKey                = "site.logo"
	siteDomainKey              = "site.domain"
	siteRegisterEnabledKey     = "site.register_enabled"
	siteRegisterDefaultRoleKey = "site.register_default_role"
)

// SiteSettingsService implements zerxv1connect.SiteSettingsServiceHandler,
// persisting site-wide presentation settings as fixed-key system parameters.
type SiteSettingsService struct {
	db    *gorm.DB
	cache *param.Cache
	media *media.Media
}

var _ zerxv1connect.SiteSettingsServiceHandler = (*SiteSettingsService)(nil)

// NewSiteSettingsService constructs the site settings handler.
func NewSiteSettingsService(db *gorm.DB, cache *param.Cache, m *media.Media) *SiteSettingsService {
	return &SiteSettingsService{db: db, cache: cache, media: m}
}

func siteRegisterEnabled(cache *param.Cache) bool {
	v, ok := cache.Get(siteRegisterEnabledKey)
	if !ok || v == "" {
		return true
	}
	return v != "false"
}

func siteRegisterDefaultRole(cache *param.Cache) string {
	v, _ := cache.Get(siteRegisterDefaultRoleKey)
	if v == "" {
		return model.RoleUser
	}
	return v
}

func boolParam(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func (s *SiteSettingsService) current(ctx context.Context) *zerxv1.SiteSettings {
	name, _ := s.cache.Get(siteNameKey)
	logo, _ := s.cache.Get(siteLogoKey)
	domain, _ := s.cache.Get(siteDomainKey)
	enabled := siteRegisterEnabled(s.cache)
	open := enabled
	if !open {
		n, err := gorm.G[model.User](s.db).Count(ctx, "id")
		if err == nil && n == 0 {
			open = true
		}
	}
	return &zerxv1.SiteSettings{
		Name:                name,
		Logo:                s.media.ResolveLogo(logo),
		Domain:              domain,
		RegisterEnabled:     enabled,
		RegisterOpen:        open,
		RegisterDefaultRole: siteRegisterDefaultRole(s.cache),
	}
}

func (s *SiteSettingsService) GetSiteSettings(ctx context.Context, _ *connect.Request[zerxv1.GetSiteSettingsRequest]) (*connect.Response[zerxv1.SiteSettings], error) {
	return connect.NewResponse(s.current(ctx)), nil
}

func (s *SiteSettingsService) UpdateSiteSettings(ctx context.Context, req *connect.Request[zerxv1.UpdateSiteSettingsRequest]) (*connect.Response[zerxv1.SiteSettings], error) {
	roleCode := req.Msg.GetRegisterDefaultRole()
	if roleCode == "" {
		roleCode = model.RoleUser
	}
	if roleCode == model.RoleAdmin {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("register default role cannot be admin"))
	}
	if _, err := gorm.G[model.Role](s.db).Where("code = ?", roleCode).First(ctx); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("register default role not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	before := s.current(ctx)
	for key, val := range map[string]string{
		siteNameKey:                req.Msg.GetName(),
		siteLogoKey:                s.media.NormalizeStored(req.Msg.GetLogo()),
		siteDomainKey:              req.Msg.GetDomain(),
		siteRegisterEnabledKey:     boolParam(req.Msg.GetRegisterEnabled()),
		siteRegisterDefaultRoleKey: roleCode,
	} {
		if err := s.cache.Set(ctx, key, val); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	audit.Record(ctx, auditJSON(map[string]any{
		"before": map[string]any{
			"name": before.Name, "logo": before.Logo, "domain": before.Domain,
			"register_enabled": before.RegisterEnabled, "register_default_role": before.RegisterDefaultRole,
		},
		"after": map[string]any{
			"name": req.Msg.GetName(), "logo": req.Msg.GetLogo(), "domain": req.Msg.GetDomain(),
			"register_enabled": req.Msg.GetRegisterEnabled(), "register_default_role": roleCode,
		},
	}))
	return connect.NewResponse(s.current(ctx)), nil
}
