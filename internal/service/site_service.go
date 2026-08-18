package service

import (
	"context"

	"connectrpc.com/connect"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/audit"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/param"
)

// Fixed parameter keys backing the site settings.
const (
	siteNameKey   = "site.name"
	siteLogoKey   = "site.logo"
	siteDomainKey = "site.domain"
)

// SiteSettingsService implements zerxv1connect.SiteSettingsServiceHandler,
// persisting site-wide presentation settings as fixed-key system parameters.
type SiteSettingsService struct {
	cache *param.Cache
	media *media.Media
}

var _ zerxv1connect.SiteSettingsServiceHandler = (*SiteSettingsService)(nil)

// NewSiteSettingsService constructs the site settings handler.
func NewSiteSettingsService(cache *param.Cache, m *media.Media) *SiteSettingsService {
	return &SiteSettingsService{cache: cache, media: m}
}

func (s *SiteSettingsService) current() *zerxv1.SiteSettings {
	name, _ := s.cache.Get(siteNameKey)
	logo, _ := s.cache.Get(siteLogoKey)
	domain, _ := s.cache.Get(siteDomainKey)
	return &zerxv1.SiteSettings{Name: name, Logo: s.media.ResolveLogo(logo), Domain: domain}
}

func (s *SiteSettingsService) GetSiteSettings(_ context.Context, _ *connect.Request[zerxv1.GetSiteSettingsRequest]) (*connect.Response[zerxv1.SiteSettings], error) {
	return connect.NewResponse(s.current()), nil
}

func (s *SiteSettingsService) UpdateSiteSettings(ctx context.Context, req *connect.Request[zerxv1.UpdateSiteSettingsRequest]) (*connect.Response[zerxv1.SiteSettings], error) {
	before := s.current()
	for key, val := range map[string]string{
		siteNameKey:   req.Msg.GetName(),
		siteLogoKey:   s.media.NormalizeStored(req.Msg.GetLogo()),
		siteDomainKey: req.Msg.GetDomain(),
	} {
		if err := s.cache.Set(ctx, key, val); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	audit.Record(ctx, auditJSON(map[string]any{
		"before": map[string]any{"name": before.Name, "logo": before.Logo, "domain": before.Domain},
		"after":  map[string]any{"name": req.Msg.GetName(), "logo": req.Msg.GetLogo(), "domain": req.Msg.GetDomain()},
	}))
	return connect.NewResponse(s.current()), nil
}
