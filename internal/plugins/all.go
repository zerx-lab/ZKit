// Package plugins is the central, explicit assembly manifest: it registers every
// compiled-in plugin with the plugin registry. Adding a plugin means adding one
// import line and one Register line here (the two anchor comments mark the
// insertion points the generator and humans use). This deliberately avoids
// init()/import-_ self-registration: explicit Register keeps the set
// traceable, statically analyzable, and free of test-time state pollution.
package plugins

import (
	"github.com/zerx-lab/zkit/internal/plugin"
	"github.com/zerx-lab/zkit/internal/plugin/impl/shop"
	// plugin-import-anchor (do not remove: new plugin imports inserted above this line)
)

// _ keeps the plugin package referenced even when zero plugins are registered
// (e.g. after the last plugin is uninstalled), so Register()'s body going empty
// never turns the import into an "imported and not used" compile error. Do not
// remove: it is the invariant that uninstalling any plugin can't break the build.
var _ = plugin.All

// Register adds every compiled-in plugin to the global registry. Called from
// cmd/server/main.go after config load and before any DB work (pure in-memory).
func Register() {
	plugin.Register(shop.New())
	// plugin-register-anchor (do not remove: new plugin registrations inserted above this line)
}
