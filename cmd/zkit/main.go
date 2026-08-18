// Command zkit scaffolds a new project from this template repository.
//
// It clones the current repository's committed files into a new directory,
// rewriting the Go module path, binary/image/volume names, the frontend
// package name, the brand display name, the default database name, and the
// localStorage key prefix. Generated protobuf code (*.pb.go, *_pb.ts) is
// copied verbatim — its embedded descriptors carry length-prefixed metadata
// that a blind text replacement would corrupt, and that metadata is inert at
// runtime (the Go package path is recovered via reflection, not the rawDesc).
// The proto package name (zerx.v1) is intentionally preserved.
//
// Usage:
//
//	zkit new <module> [dir] [--brand Name] [--db dbname] [--agent <name>]
//
// <module> is the new Go module path (e.g. github.com/acme/foo). dir defaults
// to ./<base of module>. --brand defaults to the new short name; --db
// defaults to the sanitized new short name. --agent selects one AI CLI's
// assets; it defaults to all to preserve the historical scaffold output.
//
// Only `go build` is required to compile the generated project; codegen tools
// (buf, protoc, gorm cli) are not needed at creation time.
//
// zkit also scaffolds and packages plugins via the `plugin` subcommand:
//
//	zkit plugin new <name> [field:type,...]   scaffold a plugin (proto + impl +
//	                                             frontend page + teardown SQL +
//	                                             the anchored lines in all.go)
//	zkit plugin pack <name>                   pack an installed plugin into
//	                                             <name>.zip for distribution
//
// A bare invocation prints help and the command list.
package main

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// Constants that are themselves rewritten by the brand/db replacement when this
// file is copied into a new project, so derived templates stay self-updating.
// The module path and short name are derived at runtime from go.mod (never
// hardcoded), so grandchild renames keep working.
const (
	oldBrand = "ZKit"
	oldDB    = "zerxlab"

	agentAll      agentTarget = "all"
	agentOMP      agentTarget = "omp"
	agentPi       agentTarget = "pi"
	agentClaude   agentTarget = "claude"
	agentOpenCode agentTarget = "opencode"
	agentCodex    agentTarget = "codex"
	agentNone     agentTarget = "none"
)

const agentTargetValues = "all, omp, pi, claude, opencode, codex, none"

type agentTarget string

func (a agentTarget) valid() bool {
	switch a {
	case agentAll, agentOMP, agentPi, agentClaude, agentOpenCode, agentCodex, agentNone:
		return true
	default:
		return false
	}
}

func (a *agentTarget) Set(value string) error {
	target := agentTarget(value)
	if !target.valid() {
		return fmt.Errorf("must be one of: %s", agentTargetValues)
	}
	*a = target
	return nil
}

func (a *agentTarget) String() string {
	return string(*a)
}

func (*agentTarget) Type() string {
	return "agent"
}

// templateRepoURL is the canonical template repository cloned into the per-version
// cache when this binary is installed via `go install` (not run from a checkout).
const templateRepoURL = "https://github.com/zerx-lab/ZKit.git"

// localStorageKeys are the full localStorage key literals whose "zerx." prefix
// is rewritten to "<newShort>.". Enumerated whole (not prefix-matched) so the
// proto package "zerx.v1" is never touched.
var localStorageKeys = []string{
	"zerx.theme",
	"zerx.locale",
	"zerx.accessToken",
	"zerx.refreshToken",
	"zerx.sessionId",
}

func runNew(newModule, destDir, brand, db, from string, agent agentTarget) error {
	if !agent.valid() {
		return fmt.Errorf("invalid agent target %q; must be one of: %s", agent, agentTargetValues)
	}
	if err := module.CheckPath(newModule); err != nil {
		return fmt.Errorf("invalid module path %q: %w", newModule, err)
	}

	srcRoot, err := resolveTemplate(from)
	if err != nil {
		return err
	}

	// Derive source tokens at runtime — never hardcode.
	gomod, err := os.ReadFile(filepath.Join(srcRoot, "go.mod"))
	if err != nil {
		return fmt.Errorf("reading source go.mod: %w", err)
	}
	oldModule := modfile.ModulePath(gomod)
	if oldModule == "" {
		return fmt.Errorf("could not determine source module path from go.mod")
	}
	oldShort := path.Base(oldModule)

	newShort := sanitize(path.Base(newModule))
	if newShort == "" {
		return fmt.Errorf("new module base %q yields empty short name after sanitizing", path.Base(newModule))
	}
	if brand == "" {
		brand = newShort
	}
	if db == "" {
		db = newShort
	}

	if destDir == "" {
		destDir = "./" + path.Base(newModule)
	}
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	if entries, statErr := os.ReadDir(destAbs); statErr == nil && len(entries) > 0 {
		return fmt.Errorf("destination %s exists and is not empty", destAbs)
	}

	files, err := copySet(srcRoot, destAbs)
	if err != nil {
		return err
	}
	if err := validateAgentAssets(files.paths, agent); err != nil {
		return err
	}
	files.paths = filterAgentAssets(files.paths, agent)

	r := &rewriter{
		oldModule: oldModule,
		newModule: newModule,
		oldShort:  oldShort,
		newShort:  newShort,
		oldBrand:  oldBrand,
		newBrand:  brand,
		newDB:     db,
		dbRe:      regexp.MustCompile(`\b` + regexp.QuoteMeta(oldDB) + `\b`),
	}

	for _, rel := range files.paths {
		data, readErr := files.read(rel)
		if readErr != nil {
			return fmt.Errorf("reading %s: %w", rel, readErr)
		}
		out := r.rewrite(rel, data)
		dst := filepath.Join(destAbs, rel)
		if mkErr := os.MkdirAll(filepath.Dir(dst), 0o777); mkErr != nil {
			return mkErr
		}
		if wErr := os.WriteFile(dst, out, 0o666); wErr != nil {
			return fmt.Errorf("writing %s: %w", rel, wErr)
		}
	}

	// FIX B: synthesize the embed placeholder. vite's emptyOutDir deletes it
	// locally and git never tracks it, so //go:embed all:dist would fail to
	// compile without this.
	gitkeep := filepath.Join(destAbs, "internal", "web", "dist", ".gitkeep")
	if mkErr := os.MkdirAll(filepath.Dir(gitkeep), 0o777); mkErr != nil {
		return mkErr
	}
	if wErr := os.WriteFile(gitkeep, nil, 0o666); wErr != nil {
		return wErr
	}

	printNextSteps(destDir, brand, agent)
	return nil
}

// rewriter holds the token substitutions for a single scaffold run.
type rewriter struct {
	oldModule string
	newModule string
	oldShort  string
	newShort  string
	oldBrand  string
	newBrand  string
	newDB     string
	dbRe      *regexp.Regexp
}

func (r *rewriter) rewrite(rel string, data []byte) []byte {
	if rel == "go.mod" {
		return fixGoMod(data, r.newModule)
	}
	if strings.HasSuffix(rel, ".pb.go") {
		// Verbatim: rawDesc length-prefixed descriptors must not change.
		return data
	}
	if strings.HasSuffix(rel, ".go") {
		// FIX A: AST-only import rewrite first, then brand/db text replace.
		// Never apply the short-name or module-path text replacement to .go —
		// the module path is already handled structurally via imports.
		data = fixGoImports(data, rel, r.oldModule, r.newModule)
		data = bytes.ReplaceAll(data, []byte(r.oldBrand), []byte(r.newBrand))
		data = r.dbRe.ReplaceAll(data, []byte(r.newDB))
		return data
	}
	if isBinary(data) {
		return data
	}
	// Non-.go text: longest-first token replacement.
	data = bytes.ReplaceAll(data, []byte(r.oldModule), []byte(r.newModule))
	data = bytes.ReplaceAll(data, []byte(r.oldShort), []byte(r.newShort))
	data = bytes.ReplaceAll(data, []byte(r.oldBrand), []byte(r.newBrand))
	data = r.dbRe.ReplaceAll(data, []byte(r.newDB))
	for _, key := range localStorageKeys {
		suffix := strings.TrimPrefix(key, "zerx.")
		data = bytes.ReplaceAll(data, []byte(key), []byte(r.newShort+"."+suffix))
	}
	return data
}

// edit is a byte-range replacement.
type edit struct {
	start, end int
	repl       string
}

// applyEdits splices replacements into data in descending start order so byte
// offsets outside the edits stay verbatim.
func applyEdits(data []byte, edits []edit) []byte {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := append([]byte(nil), data...)
	for _, e := range edits {
		out = append(out[:e.start], append([]byte(e.repl), out[e.end:]...)...)
	}
	return out
}

// fixGoImports rewrites only import-spec paths matching oldMod (or oldMod/...).
// Everything outside import specs (including rawDesc literals) is byte-identical.
func fixGoImports(data []byte, file, oldMod, newMod string) []byte {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, data, parser.ImportsOnly)
	if err != nil {
		// A file we cannot parse for imports is left untouched (defensive;
		// the template's .go files all parse).
		return data
	}
	at := func(p token.Pos) int { return fset.File(p).Offset(p) }
	var edits []edit
	for _, spec := range f.Imports {
		p, uErr := strconv.Unquote(spec.Path.Value)
		if uErr != nil {
			continue
		}
		switch {
		case p == oldMod:
			edits = append(edits, edit{at(spec.Path.Pos()), at(spec.Path.End()), strconv.Quote(newMod)})
		case strings.HasPrefix(p, oldMod+"/"):
			edits = append(edits, edit{at(spec.Path.Pos()), at(spec.Path.End()), strconv.Quote(newMod + strings.TrimPrefix(p, oldMod))})
		}
	}
	if len(edits) == 0 {
		return data
	}
	return applyEdits(data, edits)
}

func fixGoMod(data []byte, newMod string) []byte {
	f, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil {
		return data
	}
	if mErr := f.AddModuleStmt(newMod); mErr != nil {
		return data
	}
	out, fErr := f.Format()
	if fErr != nil {
		return data
	}
	return out
}

// fileSource enumerates the template files to copy and reads their content.
// In git mode content comes from the index (git show :path), so a copy reflects
// the tracked snapshot rather than any dirty working-tree edits; in walk mode it
// reads from disk.
type fileSource struct {
	paths []string
	read  func(rel string) ([]byte, error)
}

// copySet returns the file source to copy, preferring git-tracked files and
// falling back to a filtered directory walk. Paths inside destAbs are skipped.
func copySet(srcRoot, destAbs string) (*fileSource, error) {
	if files, err := gitFiles(srcRoot); err == nil {
		return &fileSource{
			paths: filterDest(srcRoot, destAbs, files),
			read: func(rel string) ([]byte, error) {
				return gitIndexContent(srcRoot, rel)
			},
		}, nil
	}
	paths, err := walkFiles(srcRoot, destAbs)
	if err != nil {
		return nil, err
	}
	return &fileSource{
		paths: paths,
		read: func(rel string) ([]byte, error) {
			return os.ReadFile(filepath.Join(srcRoot, rel))
		},
	}, nil
}

func gitFiles(srcRoot string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = srcRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, p := range bytes.Split(out, []byte{0}) {
		if len(p) == 0 {
			continue
		}
		files = append(files, filepath.FromSlash(string(p)))
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("git ls-files returned nothing")
	}
	return files, nil
}

// gitIndexContent reads a tracked file's content from the index (staged tree),
// matching the set returned by `git ls-files` and ignoring dirty working-tree
// edits to tracked files.
func gitIndexContent(srcRoot, rel string) ([]byte, error) {
	cmd := exec.Command("git", "show", ":"+filepath.ToSlash(rel))
	cmd.Dir = srcRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show :%s: %w", filepath.ToSlash(rel), err)
	}
	return out, nil
}

func filterDest(srcRoot, destAbs string, files []string) []string {
	var kept []string
	for _, rel := range files {
		abs := filepath.Join(srcRoot, rel)
		if within(destAbs, abs) {
			continue
		}
		kept = append(kept, rel)
	}
	return kept
}

// validateAgentAssets prevents a project that was already pruned for one CLI
// from silently claiming it can scaffold another CLI's missing configuration.
func validateAgentAssets(files []string, target agentTarget) error {
	if target == agentAll || target == agentNone {
		return nil
	}

	required := []string(nil)
	switch target {
	case agentOMP:
		required = []string{".omp"}
	case agentPi:
		required = []string{".pi", "tools/arch-guard"}
	case agentClaude:
		required = []string{".claude", "tools/arch-guard"}
	case agentOpenCode:
		required = []string{".opencode", "tools/arch-guard"}
	case agentCodex:
		if hasAgentAsset(files, "AGENTS.md") || hasAgentAsset(files, ".agents") {
			return nil
		}
		required = []string{"AGENTS.md or .agents"}
	}

	missing := required[:0]
	for _, root := range required {
		if !hasAgentAsset(files, root) {
			missing = append(missing, root)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("template does not contain required %s agent assets: %s", target, strings.Join(missing, ", "))
	}
	return nil
}

func hasAgentAsset(files []string, root string) bool {
	for _, rel := range files {
		rel = filepath.ToSlash(rel)
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}

// filterAgentAssets removes AI CLI files that do not belong to the selected
// target. Shared AGENTS.md and .agents skills accompany every concrete target;
// pi, Claude, and OpenCode also need the shared arch-guard implementation.
func filterAgentAssets(files []string, target agentTarget) []string {
	if target == agentAll {
		return files
	}
	kept := files[:0]
	for _, rel := range files {
		if keepAgentAsset(rel, target) {
			kept = append(kept, rel)
		}
	}
	return kept
}

func keepAgentAsset(rel string, target agentTarget) bool {
	if target == agentAll {
		return true
	}
	rel = filepath.ToSlash(rel)
	if rel == "AGENTS.md" || rel == ".agents" || strings.HasPrefix(rel, ".agents/") {
		return target != agentNone
	}
	if rel == ".omp" || strings.HasPrefix(rel, ".omp/") {
		return target == agentOMP
	}
	if rel == ".pi" || strings.HasPrefix(rel, ".pi/") {
		return target == agentPi
	}
	if rel == ".claude" || strings.HasPrefix(rel, ".claude/") {
		return target == agentClaude
	}
	if rel == ".opencode" || strings.HasPrefix(rel, ".opencode/") {
		return target == agentOpenCode
	}
	if rel == "tools/arch-guard" || strings.HasPrefix(rel, "tools/arch-guard/") {
		return target == agentPi || target == agentClaude || target == agentOpenCode
	}
	return true
}

var walkExclude = []string{
	".git", ".bin", "bin", "tmp", "data", "web/node_modules", "web/dist",
	"internal/web/dist", ".deps-snapshot", "web/.tanstack/tmp",
}

func walkFiles(srcRoot, destAbs string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(srcRoot, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if within(destAbs, p) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(srcRoot, p)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		slash := filepath.ToSlash(rel)
		for _, ex := range walkExclude {
			if slash == ex || strings.HasPrefix(slash, ex+"/") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if d.IsDir() {
			return nil
		}
		base := d.Name()
		if base == ".env" || strings.HasSuffix(base, ".db") ||
			strings.HasSuffix(base, ".db-shm") || strings.HasSuffix(base, ".db-wal") {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// within reports whether target is parent or equal to target's path.
func within(parent, target string) bool {
	rel, err := filepath.Rel(parent, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isBinary reports whether the first 8KB contains a NUL byte.
func isBinary(data []byte) bool {
	n := len(data)
	if n > 8192 {
		n = 8192
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}

// resolveTemplate returns the root directory of the template to copy from.
//
// Three modes:
//   - from != "": use that directory verbatim (no cache, no tag lock). This is
//     how `task new` runs inside the repo (--from <repo root>).
//   - binary version is "(devel)" / unknown: use the current working directory
//     (running `go run ./cmd/zkit` inside a checkout).
//   - installed binary (real tag or pseudo-version): clone/checkout the template
//     at that version into ~/.ZKit/<version> and use it.
func resolveTemplate(from string) (string, error) {
	if from != "" {
		abs, err := filepath.Abs(from)
		if err != nil {
			return "", err
		}
		if _, statErr := os.Stat(filepath.Join(abs, "go.mod")); statErr != nil {
			return "", fmt.Errorf("--from %s is not a template root (no go.mod): %w", abs, statErr)
		}
		return abs, nil
	}

	version := selfVersion()
	if version == "" || version == "(devel)" {
		return os.Getwd()
	}
	return ensureTemplate(version)
}

// selfVersion returns this binary's module version via build info, or "" when
// unavailable. "(devel)" indicates a local `go run`/build, not an installed tag.
func selfVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Version
}

// isPseudoVersion reports whether v is a Go pseudo-version (e.g. produced by
// `go install ...@latest` before any tag exists), as opposed to a real tag.
func isPseudoVersion(v string) bool {
	return module.IsPseudoVersion(v)
}

// ensureTemplate makes sure the template at the given version is checked out in
// the per-version cache (~/.ZKit/<version>) and returns its path.
//
// Real tags are immutable: once cloned, no network is needed. Pseudo-versions
// (no tag yet, mode A) track the remote default branch, so we fetch and reset to
// origin/<default> when reachable, falling back to the cached copy offline.
func ensureTemplate(version string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git is required to fetch the template (version %s) but was not found in PATH", version)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	cacheRoot := filepath.Join(home, ".ZKit")
	dir := filepath.Join(cacheRoot, version)

	pseudo := isPseudoVersion(version)
	cloned := isGitRepo(dir)

	if !cloned {
		if mkErr := os.MkdirAll(cacheRoot, 0o777); mkErr != nil {
			return "", mkErr
		}
		// Best-effort clean of a partial dir from a prior failed clone.
		_ = os.RemoveAll(dir)
		if pseudo {
			if cloneErr := runGit("", "clone", "--depth", "1", templateRepoURL, dir); cloneErr != nil {
				return "", fmt.Errorf("cloning template: %w", cloneErr)
			}
		} else {
			// Shallow-clone exactly the tag.
			if cloneErr := runGit("", "clone", "--depth", "1", "--branch", version, templateRepoURL, dir); cloneErr != nil {
				return "", fmt.Errorf("cloning template at tag %s: %w", version, cloneErr)
			}
		}
		return dir, nil
	}

	// Cache exists. Real tag = immutable: trust the existing checkout (offline-safe).
	if !pseudo {
		return dir, nil
	}

	// Pseudo-version (mode A): refresh to the remote default branch when online;
	// fall back to the cached copy with a warning when fetch fails (lenient).
	branch, branchErr := defaultBranch(dir)
	if branchErr != nil {
		fmt.Fprintf(os.Stderr, "zkit: warning: cannot determine default branch (%v); using cached template\n", branchErr)
		return dir, nil
	}
	if fetchErr := runGit(dir, "fetch", "--depth", "1", "origin", branch); fetchErr != nil {
		fmt.Fprintf(os.Stderr, "zkit: warning: cannot check for template updates (%v); using cached template\n", fetchErr)
		return dir, nil
	}
	// Cache is a derived, read-only mirror: hard-reset, discarding any local edits.
	if resetErr := runGit(dir, "reset", "--hard", "origin/"+branch); resetErr != nil {
		return "", fmt.Errorf("updating template: %w", resetErr)
	}
	return dir, nil
}

func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info != nil && info.IsDir()
}

// defaultBranch resolves the remote's default branch name (origin/HEAD).
func defaultBranch(dir string) (string, error) {
	out, err := gitOutput(dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		ref := strings.TrimSpace(out)
		return strings.TrimPrefix(ref, "origin/"), nil
	}
	// origin/HEAD may be unset on a shallow clone; ask the remote directly.
	out, err = gitOutput(dir, "remote", "show", "origin")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if b, ok := strings.CutPrefix(line, "HEAD branch:"); ok {
			return strings.TrimSpace(b), nil
		}
	}
	return "", fmt.Errorf("could not parse default branch from remote")
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	return string(out), err
}

func printNextSteps(dir, brand string, agent agentTarget) {
	fmt.Printf("Scaffolded %q in %s\n", brand, dir)
	if agent == agentNone {
		fmt.Println("AI agent assets: none")
	} else {
		fmt.Printf("AI agent assets: %s\n", agent)
	}
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  cd %s\n", dir)
	fmt.Println("  cp .env.example .env   # then set JWT_SECRET")
	fmt.Println("  go build ./...         # compiles offline; no codegen needed")
	fmt.Println("  (cd web && bun install && bun run build)")
	fmt.Println()
	fmt.Println("For the full dev experience (regenerates code, starts dev DB):")
	fmt.Println("  task sync && task dev")
}
