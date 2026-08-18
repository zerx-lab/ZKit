package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)

// fieldRe validates a scaffold field name: snake_case identifier, safe as both a
// proto field name and (via pascal) a Go identifier.
var fieldRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// field is a parsed name:type spec for the scaffolded message/model.
type field struct {
	Name      string // snake_case original
	GoName    string // PascalCase Go field
	JSONName  string // snake_case proto field
	ProtoType string
	GoType    string
}

type pluginData struct {
	Name      string // shop
	Pascal    string // Shop
	Module    string
	Fields    []field
	HasFields bool
}

func scaffoldPlugin(name, spec string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid plugin name %q: must match %s", name, nameRe.String())
	}

	var fields []field
	if spec != "" {
		fs, err := parseFields(spec)
		if err != nil {
			return err
		}
		fields = fs
	}

	root, err := repoRoot()
	if err != nil {
		return err
	}

	module, err := readModulePath(root)
	if err != nil {
		return err
	}

	d := pluginData{
		Name:      name,
		Pascal:    pascal(name),
		Module:    module,
		Fields:    fields,
		HasFields: len(fields) > 0,
	}

	implDir := filepath.Join(root, "internal", "plugin", "impl", name)
	webDir := filepath.Join(root, "web", "src", "plugin-components", name)
	targets := []struct {
		path string
		tmpl string
	}{
		{filepath.Join(root, "proto", "zerx", "v1", name+".proto"), protoTmpl},
		{filepath.Join(implDir, "plugin.go"), pluginTmpl},
		{filepath.Join(implDir, "model.go"), modelTmpl},
		{filepath.Join(implDir, "service.go"), serviceTmpl},
		{filepath.Join(implDir, name+"_teardown.sql"), teardownTmpl},
		{filepath.Join(webDir, d.Pascal+".tsx"), webTmpl},
		{filepath.Join(webDir, "i18n.ts"), i18nTmpl},
	}

	for _, t := range targets {
		if _, err := os.Stat(t.path); err == nil {
			return fmt.Errorf("refusing to overwrite existing file: %s", t.path)
		}
	}

	for _, t := range targets {
		if err := renderFile(t.path, t.tmpl, d); err != nil {
			return err
		}
		fmt.Println("created", rel(root, t.path))
	}

	allPath := filepath.Join(root, "internal", "plugins", "all.go")
	if err := patchAll(allPath, name, d.Module); err != nil {
		return err
	}
	fmt.Println("patched", rel(root, allPath))

	if out, err := exec.Command("gofmt", "-w", implDir, allPath).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "gofmt warning: %v: %s\n", err, out)
	}

	fmt.Printf("\nNext:\n  1. task gen          # generate proto Go/TS + connect-query hooks\n  2. fill business logic in %s\n  3. task build && restart\n", rel(root, filepath.Join(implDir, "service.go")))
	return nil
}

func parseFields(spec string) ([]field, error) {
	var out []field
	seen := make(map[string]bool)
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("bad field spec %q (want name:type)", part)
		}
		fn := strings.TrimSpace(kv[0])
		// id and name are built-in scaffold fields; skip to avoid duplicates.
		if fn == "id" || fn == "name" {
			continue
		}
		if !fieldRe.MatchString(fn) {
			return nil, fmt.Errorf("invalid field name %q: must match %s", fn, fieldRe.String())
		}
		if seen[fn] {
			return nil, fmt.Errorf("duplicate field name %q", fn)
		}
		seen[fn] = true
		pt, gt, err := mapType(kv[1])
		if err != nil {
			return nil, err
		}
		out = append(out, field{
			Name:      fn,
			GoName:    pascal(fn),
			JSONName:  fn,
			ProtoType: pt,
			GoType:    gt,
		})
	}
	return out, nil
}

func mapType(t string) (string, string, error) {
	switch t {
	case "string":
		return "string", "string", nil
	case "int", "int64":
		return "int64", "int64", nil
	case "int32":
		return "int32", "int32", nil
	case "bool":
		return "bool", "bool", nil
	case "float", "double":
		return "double", "float64", nil
	default:
		return "", "", fmt.Errorf("unsupported field type %q (use string|int|int32|bool|float)", t)
	}
}

func pascal(s string) string {
	parts := strings.Split(s, "_")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(p[1:])
	}
	return b.String()
}

// readModulePath returns the module path from go.mod at root, so generated
// imports / proto go_package are correct in forks created via `task new` (which
// rewrites the module). Never hardcode the module path.
func readModulePath(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", fmt.Errorf("module directive not found in go.mod")
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		wd, werr := os.Getwd()
		if werr != nil {
			return "", err
		}
		return wd, nil
	}
	return strings.TrimSpace(string(out)), nil
}

func rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return r
}

func renderFile(path, tmpl string, d pluginData) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	funcs := template.FuncMap{
		// add2/add3 compute proto field numbers offset past id(1)+name(2).
		"add2": func(i int) int { return i + 2 }, // create: after name=1
		"add3": func(i int) int { return i + 3 }, // entity/update: after id=1, name=2
		"addFieldBase": func(fs []field) int { return len(fs) + 3 },
	}
	t, err := template.New(filepath.Base(path)).Funcs(funcs).Parse(tmpl)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return t.Execute(f, d)
}

// patchAll inserts the import and Register lines at the anchor comments.
func patchAll(path, name, module string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := string(b)
	importLine := fmt.Sprintf("\t\"%s/internal/plugin/impl/%s\"\n", module, name)
	registerLine := fmt.Sprintf("\tplugin.Register(%s.New())\n", name)

	// Detect an existing reference by the import-path token and Register call,
	// not the newline-terminated line: the plugin on an anchor line carries a
	// trailing comment, so a "\n"-terminated needle would miss it.
	importToken := fmt.Sprintf("%s/internal/plugin/impl/%s\"", module, name)
	registerToken := fmt.Sprintf("plugin.Register(%s.New())", name)
	if strings.Contains(src, importToken) || strings.Contains(src, registerToken) {
		return fmt.Errorf("all.go already references plugin %q", name)
	}

	const importAnchor = "// plugin-import-anchor"
	const registerAnchor = "// plugin-register-anchor"
	if !strings.Contains(src, importAnchor) || !strings.Contains(src, registerAnchor) {
		return fmt.Errorf("all.go is missing the anchor comments")
	}

	// Insert import before the line carrying the import anchor.
	src = insertBeforeAnchorLine(src, importAnchor, importLine)
	src = insertBeforeAnchorLine(src, registerAnchor, registerLine)

	return os.WriteFile(path, []byte(src), 0o644)
}

// insertBeforeAnchorLine inserts line immediately before the full line that
// contains anchor (preserving the anchor line itself).
func insertBeforeAnchorLine(src, anchor, line string) string {
	lines := strings.SplitAfter(src, "\n")
	for i, l := range lines {
		if strings.Contains(l, anchor) {
			lines = append(lines[:i], append([]string{line}, lines[i:]...)...)
			break
		}
	}
	return strings.Join(lines, "")
}
