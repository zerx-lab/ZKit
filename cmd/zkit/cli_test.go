package main

import (
	"bytes"
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
