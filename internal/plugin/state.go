package plugin

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/model"
)

// State is the process-wide runtime enable/disable cache for plugins. A plugin
// is enabled unless an explicit PluginState row says otherwise (default-on).
// Disabling a plugin gates three surfaces at runtime without a rebuild:
//   - its connectRPC procedures are denied (Casbin interceptor consults this),
//   - its menus are filtered from GetUserMenus,
//   - its scheduled jobs are skipped.
//
// State is read concurrently by interceptors/handlers (RLock) and written rarely
// by the admin enable/disable RPC (Lock). It lives in the plugin package so the
// procedure->plugin reverse map is built from the same registry.
type State struct {
	db *gorm.DB

	mu       sync.RWMutex
	disabled map[string]bool // plugin name -> true when explicitly disabled

	// procToPlugin maps a connectRPC procedure to its owning plugin name. Built
	// once from the registry (immutable after construction).
	procToPlugin map[string]string
}

// NewState builds a State bound to db and loads current disabled rows. The
// procedure->plugin map is derived from registered plugins' Services(): every
// procedure under a plugin-owned service maps to that plugin. Procedures are
// matched by service prefix at query time (see PluginForProcedure).
func NewState(db *gorm.DB) (*State, error) {
	s := &State{
		db:           db,
		disabled:     make(map[string]bool),
		procToPlugin: make(map[string]string),
	}
	// service full name -> plugin name
	for _, p := range All() {
		for _, svc := range p.Services() {
			s.procToPlugin[svc] = p.Name()
		}
	}
	if err := s.Reload(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

// PluginNameOfMenu returns the owning plugin name for a menu whose Name is
// namespaced "plg_<plugin>" or "plg_<plugin>_...", or "" if it is not a
// registered plugin's menu. Matches against registered plugin names (exact or
// with a trailing "_" delimiter) so "plg_shop" cannot be mistaken for
// "plg_shopping".
func PluginNameOfMenu(menuName string) string {
	for _, p := range All() {
		prefix := "plg_" + p.Name()
		if menuName == prefix || strings.HasPrefix(menuName, prefix+"_") {
			return p.Name()
		}
	}
	return ""
}

// Reload refreshes the disabled set from the DB.
func (s *State) Reload(ctx context.Context) error {
	var rows []model.PluginState
	if err := s.db.WithContext(ctx).Where("enabled = ?", false).Find(&rows).Error; err != nil {
		return err
	}
	next := make(map[string]bool, len(rows))
	for i := range rows {
		next[rows[i].Name] = true
	}
	s.mu.Lock()
	s.disabled = next
	s.mu.Unlock()
	return nil
}

// StartReloader periodically reloads the disabled set from the DB until ctx is
// done. It is a per-instance goroutine (never a ScheduledJob: the distributed
// lock would let only one replica refresh). Required on multi-replica drivers so
// an enable/disable performed on one replica propagates to the others; a single
// node stays correct via the in-process SetEnabled update alone.
func (s *State) StartReloader(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.Reload(ctx) // best-effort; keep last values on error, retry next tick
		}
	}
}

// IsEnabled reports whether the named plugin is enabled (default-on).
func (s *State) IsEnabled(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.disabled[name]
}

// pluginForProcedure returns the owning plugin name for a connectRPC procedure
// ("/<service>/<method>"), or "" if the procedure is not plugin-owned.
func (s *State) pluginForProcedure(proc string) string {
	svc, ok := procServiceOf(proc)
	if !ok {
		return ""
	}
	return s.procToPlugin[svc]
}

// IsProcedureEnabled reports whether a procedure may be served: true for any
// non-plugin (core) procedure, and for plugin procedures only when the owning
// plugin is enabled.
func (s *State) IsProcedureEnabled(proc string) bool {
	name := s.pluginForProcedure(proc)
	if name == "" {
		return true // core procedure: not gated here
	}
	return s.IsEnabled(name)
}

// IsJobHandlerEnabled reports whether a scheduler handler key may run: true for
// core handlers, and for plugin handlers only when the owning plugin is enabled.
// Plugin job keys are prefixed "<name>_"; matched against registered plugins so
// an unrelated core handler is never gated.
func (s *State) IsJobHandlerEnabled(handler string) bool {
	for _, p := range All() {
		if _, ok := p.JobHandlers()[handler]; ok {
			return s.IsEnabled(p.Name())
		}
	}
	return true // not a plugin job
}

// EnabledPublicPage is a public page belonging to an enabled plugin.
type EnabledPublicPage struct {
	Path      string
	Component string
	Title     string
}

// EnabledPublicPages returns the public pages of every currently-enabled plugin.
// Used by the anonymous ListPublicPages endpoint so the /pub/$ route renders only
// enabled plugins' pages (a disabled plugin's public pages disappear).
func (s *State) EnabledPublicPages() []EnabledPublicPage {
	var out []EnabledPublicPage
	for _, p := range All() {
		if !s.IsEnabled(p.Name()) {
			continue
		}
		for _, pg := range p.PublicPages() {
			out = append(out, EnabledPublicPage(pg))
		}
	}
	return out
}

// SetEnabled persists the enable/disable state for a plugin and updates the
// cache. Unknown plugin names are rejected by the caller (PluginService).
func (s *State) SetEnabled(ctx context.Context, name string, enabled bool) error {
	// Upsert via explicit column maps: a zero-value bool (enabled=false) must be
	// written verbatim, so neither a struct insert (the column's default:true
	// wins) nor a struct Updates (zero values skipped) is safe here.
	now := time.Now()
	var existing model.PluginState
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := s.db.WithContext(ctx).Model(&model.PluginState{}).Create(map[string]any{
			"name": name, "enabled": enabled, "updated_at": now,
		}).Error; err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if err := s.db.WithContext(ctx).Model(&model.PluginState{}).Where("name = ?", name).Updates(map[string]any{
			"enabled": enabled, "updated_at": now,
		}).Error; err != nil {
			return err
		}
	}
	s.mu.Lock()
	if enabled {
		delete(s.disabled, name)
	} else {
		s.disabled[name] = true
	}
	s.mu.Unlock()
	return nil
}
