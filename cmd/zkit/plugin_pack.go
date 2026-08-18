package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// packPlugin builds a distributable <name>.zip from an installed plugin's source
// (proto + impl + web), in the layout internal/plugin/installer expects:
//
//	plugin.json            {"name":"<name>"}
//	proto/<name>.proto
//	impl/...               (internal/plugin/impl/<name>/*)
//	web/...                (web/src/plugin-components/<name>/*)
func packPlugin(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid plugin name %q", name)
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	implDir := filepath.Join(root, "internal", "plugin", "impl", name)
	webDir := filepath.Join(root, "web", "src", "plugin-components", name)
	protoPath := filepath.Join(root, "proto", "zerx", "v1", name+".proto")
	if _, err := os.Stat(implDir); err != nil {
		return fmt.Errorf("plugin %q not found (%s)", name, implDir)
	}

	out := filepath.Join(root, name+".zip")
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)

	add := func(arcPath string, data []byte) error {
		w, err := zw.Create(arcPath)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}

	manifest, _ := json.MarshalIndent(map[string]string{"name": name}, "", "  ")
	if err := add("plugin.json", manifest); err != nil {
		return err
	}
	if b, err := os.ReadFile(protoPath); err == nil {
		if err := add("proto/"+name+".proto", b); err != nil {
			return err
		}
	}
	if err := addTree(zw, implDir, "impl"); err != nil {
		return err
	}
	if err := addTree(zw, webDir, "web"); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	fmt.Printf("packed %s\n", out)
	return nil
}

// addTree writes every file under dir into the zip under prefix/.
func addTree(zw *zip.Writer, dir, prefix string) error {
	if _, err := os.Stat(dir); err != nil {
		return nil // optional tree (e.g. web-less plugin)
	}
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		w, err := zw.Create(prefix + "/" + strings.ReplaceAll(rel, "\\", "/"))
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	})
}
