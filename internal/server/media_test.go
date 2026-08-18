package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/database"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/storage"
)

func get(t *testing.T, h http.Handler, target, bearer string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func signedPath(m *media.Media, key string) string {
	u := m.ResolveFile(key, model.VisibilityPrivate)
	pu, err := url.Parse(u)
	if err != nil {
		return u
	}
	return pu.Path + "?" + pu.RawQuery
}

func TestMediaHandlerAuthorization(t *testing.T) {
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(db, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	dir := t.TempDir()
	scfg := config.StorageConfig{Driver: "local", LocalDir: dir, LocalBaseURL: "/uploads", SignedURLTTL: time.Hour}
	store, err := storage.New(scfg)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	m := media.New(store, scfg, []byte("test-sign-key"))
	issuer := auth.NewIssuer(config.JWTConfig{Secret: "test", AccessTTL: time.Minute, RefreshTTL: time.Hour})
	h := mediaHandler(issuer, m, db, "/uploads")
	ctx := context.Background()

	files := []struct {
		key, vis string
		owner    uint64
	}{
		{"pub.txt", model.VisibilityPublic, 1},
		{"auth.txt", model.VisibilityAuthenticated, 1},
		{"priv.txt", model.VisibilityPrivate, 1},
	}
	for _, f := range files {
		if err := store.Save(ctx, f.key, strings.NewReader("data"), 4, "text/plain"); err != nil {
			t.Fatalf("save %s: %v", f.key, err)
		}
		rec := model.File{Name: f.key, Key: f.key, ContentType: "text/plain", Visibility: f.vis, UploadedBy: f.owner}
		if err := gorm.G[model.File](db).Create(ctx, &rec); err != nil {
			t.Fatalf("create row %s: %v", f.key, err)
		}
	}

	ownerTok, _ := issuer.IssueAccess(1, []string{"user"})
	otherTok, _ := issuer.IssueAccess(2, []string{"user"})
	adminTok, _ := issuer.IssueAccess(3, []string{model.RoleAdmin})

	cases := []struct {
		name, target, bearer string
		want                 int
	}{
		{"public anon", "/uploads/pub.txt", "", http.StatusOK},
		{"auth anon", "/uploads/auth.txt", "", http.StatusUnauthorized},
		{"auth signed", signedPath(m, "auth.txt"), "", http.StatusOK},
		{"auth bearer", "/uploads/auth.txt", otherTok, http.StatusOK},
		{"private other", "/uploads/priv.txt", otherTok, http.StatusForbidden},
		{"private owner", "/uploads/priv.txt", ownerTok, http.StatusOK},
		{"private admin", "/uploads/priv.txt", adminTok, http.StatusOK},
		{"private signed", signedPath(m, "priv.txt"), "", http.StatusOK},
		{"missing", "/uploads/missing.txt", adminTok, http.StatusNotFound},
	}
	for _, c := range cases {
		if got := get(t, h, c.target, c.bearer); got != c.want {
			t.Errorf("%s = %d, want %d", c.name, got, c.want)
		}
	}
}
