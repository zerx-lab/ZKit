package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// dist is embedded at compile time, so the handler's behaviour depends on
// whether the frontend has been built. Each test asserts the branch matching
// the current embedded content.
func distBuilt(t *testing.T) (fs.FS, bool) {
	t.Helper()
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	_, err = fs.Stat(sub, "index.html")
	return sub, err == nil
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestSPAHandlerUnknownAssetFallsBackToIndex(t *testing.T) {
	sub, built := distBuilt(t)
	h := SPAHandler()
	rec := get(t, h, "/assets/does-not-exist.js")

	if !built {
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("unbuilt: status = %d, want 503", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "task build") {
			t.Fatalf("unbuilt: body lacks build hint: %q", rec.Body.String())
		}
		return
	}

	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache (fallback must not be immutable)", got)
	}
	if rec.Body.String() != string(index) {
		t.Fatal("fallback body is not index.html")
	}
}

func TestSPAHandlerClientRouteFallsBackToIndex(t *testing.T) {
	sub, built := distBuilt(t)
	h := SPAHandler()

	for _, target := range []string{"/", "/users/42", "/deep/nested/route?x=1"} {
		rec := get(t, h, target)
		if !built {
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("%s unbuilt: status = %d, want 503", target, rec.Code)
			}
			continue
		}
		index, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			t.Fatalf("read index.html: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", target, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("%s: Cache-Control = %q, want no-cache", target, got)
		}
		if rec.Body.String() != string(index) {
			t.Fatalf("%s: body is not index.html", target)
		}
	}
}

func TestSPAHandlerHashedAssetsAreImmutable(t *testing.T) {
	sub, built := distBuilt(t)
	if !built {
		// Placeholder-only dist: every request must be a 503 with the hint,
		// and the placeholder itself must not be served.
		rec := get(t, SPAHandler(), "/.gitkeep")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("unbuilt: status = %d, want 503", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "task build") {
			t.Fatalf("unbuilt: body lacks build hint: %q", rec.Body.String())
		}
		return
	}

	assets, err := fs.Glob(sub, "assets/*")
	if err != nil {
		t.Fatalf("glob assets: %v", err)
	}
	if len(assets) == 0 {
		t.Skip("built dist has no assets/ entries")
	}

	h := SPAHandler()
	rec := get(t, h, "/"+assets[0])
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d, want 200", assets[0], rec.Code)
	}
	want := "public, max-age=31536000, immutable"
	if got := rec.Header().Get("Cache-Control"); got != want {
		t.Fatalf("%s: Cache-Control = %q, want %q", assets[0], got, want)
	}
	body, err := fs.ReadFile(sub, assets[0])
	if err != nil {
		t.Fatalf("read %s: %v", assets[0], err)
	}
	if rec.Body.String() != string(body) {
		t.Fatalf("%s: served body differs from embedded file", assets[0])
	}
}

func TestSPAHandlerDirectoryPathFallsBackToIndex(t *testing.T) {
	sub, built := distBuilt(t)
	if !built {
		t.Skip("dist not built; directory fallback only observable with a built SPA")
	}
	entries, err := fs.ReadDir(sub, ".")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var dir string
	for _, e := range entries {
		if e.IsDir() {
			dir = e.Name()
			break
		}
	}
	if dir == "" {
		t.Skip("built dist has no subdirectories")
	}

	rec := get(t, SPAHandler(), "/"+dir+"/")
	if rec.Code != http.StatusOK {
		t.Fatalf("/%s/: status = %d, want 200", dir, rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("/%s/: Cache-Control = %q, want no-cache (directories must not be listed)", dir, got)
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if rec.Body.String() != string(index) {
		t.Fatalf("/%s/: body is not index.html", dir)
	}
}

func TestSetCacheHeaders(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"assets/app-abc123.js", "public, max-age=31536000, immutable"},
		{"index.html", "no-cache"},
		{"favicon.ico", ""},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		setCacheHeaders(rec, tc.name)
		if got := rec.Header().Get("Cache-Control"); got != tc.want {
			t.Errorf("setCacheHeaders(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}
