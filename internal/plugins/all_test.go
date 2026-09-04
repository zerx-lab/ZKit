package plugins

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zerx-lab/zkit/internal/database"
	"github.com/zerx-lab/zkit/internal/plugin"
)

// registerOnce guards the global registry: Register() appends, so calling it
// from every test would register duplicates and trip ValidateAll.
var registerOnce sync.Once

// componentsRoot is the front-end glob root, relative to this test file.
// Mirrors import.meta.glob("/src/plugin-components/**/*.tsx") in
// web/src/routes/_authed/p.$.tsx and web/src/routes/pub.$.tsx, which resolve a
// component id as `/src/plugin-components/${component}.tsx`.
const componentsRoot = "../../web/src/plugin-components"

// TestRegisteredPluginsValidate asserts the compiled-in registry passes the same
// startup validation cmd/server/main.go runs (naming, namespacing, procedure
// ownership, core-menu collisions), so a bad all.go fails in `go test` rather
// than at boot.
func TestRegisteredPluginsValidate(t *testing.T) {
	registerOnce.Do(Register)
	names, paths := database.ReservedMenuIdentifiers()
	if err := plugin.ValidateAll(plugin.Reserved{MenuNames: names, MenuPaths: paths}); err != nil {
		t.Fatalf("ValidateAll: %v", err)
	}
}

// TestRegisteredPluginComponentsExist asserts every Component id a registered
// plugin declares (SeedMenus subtree + PublicPages) maps to an existing
// web/src/plugin-components/<id>.tsx file. The front-end glob is compile-time,
// so a dangling id would only surface as a NotFound page in the browser.
func TestRegisteredPluginComponentsExist(t *testing.T) {
	registerOnce.Do(Register)
	plugins := plugin.All()
	checked := 0
	for _, p := range plugins {
		check := func(kind, component string) {
			if component == "" {
				return
			}
			checked++
			want := filepath.Join(componentsRoot, component+".tsx")
			if _, err := os.Stat(want); err != nil {
				t.Errorf("plugin %q %s component %q: expected file %s: %v", p.Name(), kind, component, want, err)
			}
		}
		var walk func(nodes []plugin.MenuNode)
		walk = func(nodes []plugin.MenuNode) {
			for i := range nodes {
				check("menu", nodes[i].Component)
				walk(nodes[i].Children)
			}
		}
		walk(p.SeedMenus())
		for _, pg := range p.PublicPages() {
			check("public page", pg.Component)
		}
	}
	t.Logf("checked %d component(s) across %d plugin(s)", checked, len(plugins))
}
