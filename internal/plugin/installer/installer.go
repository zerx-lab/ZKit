// Package installer unpacks uploaded plugin source packages into the repo and
// wires them into internal/plugins/all.go (install), or removes them (uninstall).
//
// This is the scaffold-style install of a COMPILE-TIME plugin system: it writes
// Go/proto/TSX source to disk and edits all.go, but never runs the new code in
// the current process. A rebuild + restart applies the change (air auto-restarts
// in dev). It is therefore admin-only and gated by config (off in prod by
// default) because it writes executable source into the repo.
package installer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)

// goKeywords are rejected as plugin names: they are injected verbatim into
// all.go as an import alias and `plugin.Register(<name>.New())`, so a keyword
// would produce uncompilable Go.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

const (
	importAnchor   = "// plugin-import-anchor"
	registerAnchor = "// plugin-register-anchor"

	maxFileBytes  = 5 << 20  // 5 MB per source file
	maxTotalBytes = 25 << 20 // 25 MB total uncompressed
	maxFileCount  = 256      // entry-count cap (zip-bomb / inode guard)
)

// validName reports whether a plugin name is syntactically valid AND not a Go
// keyword (used by both install and uninstall).
func validName(name string) bool {
	return nameRe.MatchString(name) && !goKeywords[name]
}

// Manifest is the plugin.json at the root of a plugin package.
type Manifest struct {
	Name string `json:"name"`
}

// Result describes a completed install/uninstall.
type Result struct {
	Name string
}

// Install unpacks a plugin zip (bytes) into root and patches all.go. module is
// the Go module path (for the all.go import line). Returns the plugin name.
//
// Package layout (produced by `task plugin-pack`):
//
//	plugin.json                      {"name":"<name>"}
//	proto/<name>.proto
//	impl/<file>.go ...               -> internal/plugin/impl/<name>/
//	web/<file>.tsx ...               -> web/src/plugin-components/<name>/
func Install(zipBytes []byte, root, module string) (Result, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return Result{}, fmt.Errorf("read zip: %w", err)
	}

	name, files, err := readPackage(zr)
	if err != nil {
		return Result{}, err
	}

	implDir := filepath.Join(root, "internal", "plugin", "impl", name)
	webDir := filepath.Join(root, "web", "src", "plugin-components", name)
	protoPath := filepath.Join(root, "proto", "zerx", "v1", name+".proto")

	// Refuse to clobber an existing plugin.
	for _, p := range []string{implDir, webDir, protoPath} {
		if _, err := os.Stat(p); err == nil {
			return Result{}, fmt.Errorf("plugin %q already present at %s", name, p)
		}
	}
	allPath := filepath.Join(root, "internal", "plugins", "all.go")
	if err := assertAllPatchable(allPath, name, module); err != nil {
		return Result{}, err
	}

	// Map archive entries to destination paths, then write.
	type out struct {
		path string
		data []byte
	}
	var writes []out
	for arcPath, data := range files {
		dest, ok := destFor(arcPath, name, root, protoPath, implDir, webDir)
		if !ok {
			continue // plugin.json and unrecognized entries skipped
		}
		writes = append(writes, out{dest, data})
	}
	if len(writes) == 0 {
		return Result{}, fmt.Errorf("package has no installable files")
	}

	written := make([]string, 0, len(writes))
	cleanup := func() {
		for _, w := range written {
			_ = os.Remove(w)
		}
		// RemoveAll: nested subdirs may remain after per-file removal, and a
		// leftover dir would trip the clobber check on the next attempt.
		_ = os.RemoveAll(implDir)
		_ = os.RemoveAll(webDir)
	}
	for _, w := range writes {
		if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
			cleanup()
			return Result{}, err
		}
		if err := os.WriteFile(w.path, w.data, 0o644); err != nil {
			cleanup()
			return Result{}, fmt.Errorf("write %s: %w", w.path, err)
		}
		written = append(written, w.path)
	}

	if err := patchAllInsert(allPath, name, module); err != nil {
		cleanup()
		return Result{}, err
	}

	return Result{Name: name}, nil
}

// Uninstall removes a plugin's source files and its all.go lines. Returns the
// teardown SQL (caller runs it if purging data).
func Uninstall(root, module, name string) (Result, error) {
	if !validName(name) {
		return Result{}, fmt.Errorf("invalid plugin name %q", name)
	}
	implDir := filepath.Join(root, "internal", "plugin", "impl", name)
	webDir := filepath.Join(root, "web", "src", "plugin-components", name)
	protoPath := filepath.Join(root, "proto", "zerx", "v1", name+".proto")

	// Patch all.go FIRST. While the plugin's impl/<name>/*.go is still imported,
	// a live `air` build holds open handles to those files; on Windows that makes
	// RemoveAll fail with "being used by another process". Removing the import +
	// Register lines lets the next rebuild drop the package, which releases the
	// handles. Doing this first also guarantees all.go is left compilable even if
	// a later file removal fails (the old order left a dangling import on error).
	if err := patchAllRemove(filepath.Join(root, "internal", "plugins", "all.go"), name, module); err != nil {
		return Result{}, err
	}

	// removePath retries to absorb the Windows handle-release race: the rebuild
	// triggered by the all.go edit runs concurrently, so the lock may clear a
	// moment later. A residual empty dir after all retries is tolerated
	// (Install's clobber check + a process restart clean it up); the build is
	// already correct from the patch above, which is the load-bearing invariant.
	for _, p := range []string{implDir, webDir} {
		if err := removePath(p); err != nil {
			return Result{}, fmt.Errorf("remove %s: %w", p, err)
		}
	}
	if err := removePath(protoPath); err != nil {
		return Result{}, fmt.Errorf("remove %s: %w", protoPath, err)
	}
	return Result{Name: name}, nil
}

// removePath deletes a file or directory tree, retrying briefly to ride out
// transient OS locks (Windows holds handles on .go files a live build just
// compiled until the rebuild that drops them lands). Absent files are not an
// error. If only an empty directory remains after the last attempt, that is
// tolerated: it is harmless residue, not a failed uninstall.
func removePath(p string) error {
	var err error
	for i := 0; i < 10; i++ {
		if err = os.RemoveAll(p); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	// Last resort: if the tree is gone except for empty dirs we could not unlink
	// (a locked-but-emptied directory), treat it as success \u2014 no source remains.
	if onlyEmptyDirs(p) {
		return nil
	}
	return err
}

// onlyEmptyDirs reports whether p is absent, or is a directory tree containing
// no files (only empty subdirectories). Used to accept a locked-but-emptied
// directory as a successful removal.
func onlyEmptyDirs(p string) bool {
	info, err := os.Stat(p)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil || !info.IsDir() {
		return false
	}
	empty := true
	_ = filepath.WalkDir(p, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			empty = false
		}
		return nil
	})
	return empty
}

// readPackage validates the manifest + entries and returns the name and a map of
// archive-relative path -> contents. Enforces name regex and zip-slip safety.
func readPackage(zr *zip.Reader) (string, map[string][]byte, error) {
	files := make(map[string][]byte)
	var manifestRaw []byte
	var total int64
	count := 0
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		count++
		if count > maxFileCount {
			return "", nil, fmt.Errorf("package has too many files (max %d)", maxFileCount)
		}
		clean := cleanArcPath(f.Name)
		if clean == "" {
			return "", nil, fmt.Errorf("unsafe path in package: %q", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return "", nil, err
		}
		// Read one byte past the per-file cap to detect (and reject, not silently
		// truncate) an oversize file.
		data, err := io.ReadAll(io.LimitReader(rc, maxFileBytes+1))
		_ = rc.Close()
		if err != nil {
			return "", nil, err
		}
		if len(data) > maxFileBytes {
			return "", nil, fmt.Errorf("file %q exceeds %d bytes", clean, maxFileBytes)
		}
		total += int64(len(data))
		if total > maxTotalBytes {
			return "", nil, fmt.Errorf("package exceeds %d bytes total", maxTotalBytes)
		}
		if clean == "plugin.json" {
			manifestRaw = data
			continue
		}
		files[clean] = data
	}
	if manifestRaw == nil {
		return "", nil, fmt.Errorf("package missing plugin.json")
	}
	var m Manifest
	if err := json.Unmarshal(manifestRaw, &m); err != nil {
		return "", nil, fmt.Errorf("parse plugin.json: %w", err)
	}
	if !validName(m.Name) {
		return "", nil, fmt.Errorf("invalid plugin name %q in plugin.json (bad chars or Go keyword)", m.Name)
	}
	return m.Name, files, nil
}

// cleanArcPath normalizes a zip entry path and rejects traversal/absolute paths
// (zip-slip). Returns "" if unsafe.
func cleanArcPath(name string) string {
	p := strings.ReplaceAll(name, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "../") || strings.Contains(p, "/../") || p == ".." {
		return ""
	}
	cleaned := filepath.ToSlash(filepath.Clean(p))
	if cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return cleaned
}

// destFor maps an archive path to its on-disk destination, or ok=false to skip.
func destFor(arc, name, root, protoPath, implDir, webDir string) (string, bool) {
	switch {
	case arc == "proto/"+name+".proto":
		return protoPath, true
	case strings.HasPrefix(arc, "impl/"):
		return filepath.Join(implDir, filepath.FromSlash(strings.TrimPrefix(arc, "impl/"))), true
	case strings.HasPrefix(arc, "web/"):
		return filepath.Join(webDir, filepath.FromSlash(strings.TrimPrefix(arc, "web/"))), true
	default:
		return "", false
	}
}

// assertAllPatchable verifies all.go has the anchors and does not already
// reference the plugin.
func assertAllPatchable(path, name, module string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := string(b)
	if !strings.Contains(src, importAnchor) || !strings.Contains(src, registerAnchor) {
		return fmt.Errorf("all.go is missing the anchor comments")
	}
	if strings.Contains(src, fmt.Sprintf("%s/internal/plugin/impl/%s\"", module, name)) ||
		strings.Contains(src, fmt.Sprintf("plugin.Register(%s.New())", name)) {
		return fmt.Errorf("all.go already references plugin %q", name)
	}
	return nil
}

// writeAllGo validates that src is parseable Go before writing it to path. A
// malformed all.go would break the build for the whole binary, so we refuse to
// persist one and return an error instead (the on-disk file stays valid).
func writeAllGo(path, src string) error {
	if _, err := parser.ParseFile(token.NewFileSet(), "all.go", src, parser.AllErrors); err != nil {
		return fmt.Errorf("refusing to write malformed all.go: %w", err)
	}
	return os.WriteFile(path, []byte(src), 0o644) //nolint:gosec // G703: path is <root>/internal/plugins/all.go, not user input
}

func patchAllInsert(path, name, module string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := string(b)
	importLine := fmt.Sprintf("\t\"%s/internal/plugin/impl/%s\"\n", module, name)
	registerLine := fmt.Sprintf("\tplugin.Register(%s.New())\n", name)
	src = insertBeforeAnchor(src, importAnchor, importLine)
	src = insertBeforeAnchor(src, registerAnchor, registerLine)
	return writeAllGo(path, src)
}

func patchAllRemove(path, name, module string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	importToken := fmt.Sprintf("%s/internal/plugin/impl/%s\"", module, name)
	registerToken := fmt.Sprintf("plugin.Register(%s.New())", name)
	var kept []string
	for _, line := range strings.SplitAfter(string(b), "\n") {
		// Never drop the anchor-bearing line even if it also matches.
		if strings.Contains(line, importAnchor) || strings.Contains(line, registerAnchor) {
			kept = append(kept, line)
			continue
		}
		if strings.Contains(line, importToken) || strings.Contains(line, registerToken) {
			continue
		}
		kept = append(kept, line)
	}
	return writeAllGo(path, strings.Join(kept, ""))
}

func insertBeforeAnchor(src, anchor, line string) string {
	lines := strings.SplitAfter(src, "\n")
	for i, l := range lines {
		if strings.Contains(l, anchor) {
			lines = append(lines[:i], append([]string{line}, lines[i:]...)...)
			break
		}
	}
	return strings.Join(lines, "")
}
