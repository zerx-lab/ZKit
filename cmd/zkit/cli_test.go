package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootBareShowsCommandList(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{})
	if err := root.Execute(); err != nil {
		t.Fatalf("bare invocation returned error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"Available Commands", "new", "plugin"} {
		if !strings.Contains(got, want) {
			t.Errorf("help output missing %q; got:\n%s", want, got)
		}
	}
}

func TestUnknownCommandErrors(t *testing.T) {
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"frobnicate"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("expected unknown command error, got: %v", err)
	}
}

func TestNewRequiresModuleArg(t *testing.T) {
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"new"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error when 'new' has no module arg")
	}
}

func TestPluginListsSubcommands(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"plugin"})
	if err := root.Execute(); err != nil {
		t.Fatalf("plugin invocation returned error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"new", "pack"} {
		if !strings.Contains(got, want) {
			t.Errorf("plugin help missing %q; got:\n%s", want, got)
		}
	}
}

func TestNewAgentFlagRejectsUnknownTarget(t *testing.T) {
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"new", "example.com/acme", "--agent", "cursor"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unknown agent target")
	}
	for _, want := range []string{"invalid argument", agentTargetValues} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("agent error missing %q; got: %v", want, err)
		}
	}
}

func TestFilterAgentAssets(t *testing.T) {
	files := []string{
		"README.md",
		"AGENTS.md",
		filepath.FromSlash(".agents/skills/zerx/SKILL.md"),
		filepath.FromSlash(".omp/config.yml"),
		filepath.FromSlash(".pi/settings.json"),
		filepath.FromSlash(".claude/settings.json"),
		filepath.FromSlash(".opencode/plugins/arch-guard.ts"),
		filepath.FromSlash("tools/arch-guard/match.mjs"),
		filepath.FromSlash("tools/release/main.go"),
	}
	tests := []struct {
		target agentTarget
		want   []string
	}{
		{agentAll, files},
		{agentOMP, filesAt(files, 0, 1, 2, 3, 8)},
		{agentPi, filesAt(files, 0, 1, 2, 4, 7, 8)},
		{agentClaude, filesAt(files, 0, 1, 2, 5, 7, 8)},
		{agentOpenCode, filesAt(files, 0, 1, 2, 6, 7, 8)},
		{agentCodex, filesAt(files, 0, 1, 2, 8)},
		{agentNone, filesAt(files, 0, 8)},
	}
	for _, tt := range tests {
		t.Run(string(tt.target), func(t *testing.T) {
			got := filterAgentAssets(append([]string(nil), files...), tt.target)
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Fatalf("filtered paths:\n%v\nwant:\n%v", got, tt.want)
			}
		})
	}
}

func TestRunNewAppliesAgentFilter(t *testing.T) {
	src := t.TempDir()
	writeTemplateFile(t, src, "go.mod", "module example.com/template\n\ngo 1.26\n")
	for _, rel := range []string{
		"README.md",
		"AGENTS.md",
		".agents/skills/zerx/SKILL.md",
		".omp/config.yml",
		".pi/settings.json",
		".claude/settings.json",
		".opencode/plugins/arch-guard.ts",
		"tools/arch-guard/match.mjs",
	} {
		writeTemplateFile(t, src, rel, rel)
	}

	dest := filepath.Join(t.TempDir(), "generated")
	if err := runNew("example.com/generated", dest, "", "", src, agentPi); err != nil {
		t.Fatalf("runNew: %v", err)
	}
	for _, rel := range []string{
		"README.md",
		"AGENTS.md",
		".agents/skills/zerx/SKILL.md",
		".pi/settings.json",
		"tools/arch-guard/match.mjs",
	} {
		assertPathExists(t, dest, rel, true)
	}
	for _, rel := range []string{
		".omp/config.yml",
		".claude/settings.json",
		".opencode/plugins/arch-guard.ts",
	} {
		assertPathExists(t, dest, rel, false)
	}
}

func TestRunNewRejectsUnavailableAgentAssets(t *testing.T) {
	src := t.TempDir()
	writeTemplateFile(t, src, "go.mod", "module example.com/template\n\ngo 1.26\n")
	writeTemplateFile(t, src, "AGENTS.md", "shared")
	writeTemplateFile(t, src, ".omp/config.yml", "omp")

	dest := filepath.Join(t.TempDir(), "generated")
	err := runNew("example.com/generated", dest, "", "", src, agentPi)
	if err == nil {
		t.Fatal("expected unavailable pi assets to fail")
	}
	for _, want := range []string{"required pi agent assets", ".pi", "tools/arch-guard"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("unavailable-agent error missing %q; got: %v", want, err)
		}
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("failed scaffold should not create destination, got: %v", statErr)
	}
}

func filesAt(files []string, indexes ...int) []string {
	selected := make([]string, 0, len(indexes))
	for _, index := range indexes {
		selected = append(selected, files[index])
	}
	return selected
}

func writeTemplateFile(t *testing.T, root, rel, content string) {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", rel, err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func assertPathExists(t *testing.T, root, rel string, want bool) {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	if want && err != nil {
		t.Errorf("expected %s to exist: %v", rel, err)
	}
	if !want && !os.IsNotExist(err) {
		t.Errorf("expected %s to be absent, got: %v", rel, err)
	}
}
