package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/database"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/storage"
)

func newUploadHandler(t *testing.T) (http.Handler, storage.Storage, string) {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(db, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	scfg := config.StorageConfig{Driver: "local", LocalDir: t.TempDir(), LocalBaseURL: "/uploads", SignedURLTTL: time.Hour}
	store, err := storage.New(scfg)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	m := media.New(store, scfg, []byte("test-sign-key"))
	issuer := auth.NewIssuer(config.JWTConfig{Secret: "test", AccessTTL: time.Minute, RefreshTTL: time.Hour})
	tok, err := issuer.IssueAccess(1, []string{"user"})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return uploadHandler(issuer, store, m, db), store, tok
}

func postUpload(t *testing.T, h http.Handler, bearer, filename, body string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte(body)); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestUploadHandlerContentSniffing(t *testing.T) {
	h, _, tok := newUploadHandler(t)

	cases := []struct {
		name, filename, body string
		want                 int
		wantMsg              string
	}{
		{"png with html body", "evil.png", "<html><script>alert(1)</script></html>", http.StatusBadRequest, "unsupported file content"},
		{"real png", "ok.png", "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", http.StatusOK, ""},
		{"svg with script", "evil.svg", "<svg xmlns='http://www.w3.org/2000/svg'><script>alert(1)</script></svg>", http.StatusBadRequest, "unsupported file content"},
		{"svg with xml prologue", "ok.svg", "<?xml version=\"1.0\"?>\n<svg xmlns='http://www.w3.org/2000/svg'/>", http.StatusOK, ""},
		{"svg bare with bom", "bom.svg", "\xef\xbb\xbf  <SVG xmlns='http://www.w3.org/2000/svg'/>", http.StatusOK, ""},
		{"svg without marker", "nosvg.svg", "just text", http.StatusBadRequest, "unsupported file content"},
		{"txt with html", "note.txt", "<html>x</html>", http.StatusOK, ""},
		{"no ext html", "README", "<!DOCTYPE html><html></html>", http.StatusBadRequest, "unsupported file content"},
		{"no ext text", "README", "plain", http.StatusOK, ""},
		{"zip as docx", "doc.docx", "PK\x03\x04rest", http.StatusOK, ""},
		{"exe as zip", "a.zip", "MZ\x90\x00", http.StatusBadRequest, "unsupported file content"},
		{"blocked ext", "a.exe", "MZ\x90\x00", http.StatusBadRequest, "unsupported file type"},
	}
	for _, c := range cases {
		rec := postUpload(t, h, tok, c.filename, c.body)
		if rec.Code != c.want {
			t.Errorf("%s: status = %d, want %d (body %q)", c.name, rec.Code, c.want, rec.Body.String())
			continue
		}
		if c.wantMsg != "" && !strings.Contains(rec.Body.String(), c.wantMsg) {
			t.Errorf("%s: body = %q, want %q", c.name, rec.Body.String(), c.wantMsg)
		}
	}
}

// TestUploadHandlerStoresFullContent guards the rewind after sniffing: the
// stored blob must be byte-identical to the upload, not the post-sniff tail.
func TestUploadHandlerStoresFullContent(t *testing.T) {
	h, store, tok := newUploadHandler(t)
	body := "\x89PNG\r\n\x1a\n" + strings.Repeat("x", sniffLen*2)
	rec := postUpload(t, h, tok, "big.png", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var resp struct {
		Key  string `json:"key"`
		Size int64  `json:"size"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Size != int64(len(body)) {
		t.Errorf("size = %d, want %d", resp.Size, len(body))
	}
	rc, _, err := store.Open(context.Background(), resp.Key)
	if err != nil {
		t.Fatalf("open stored blob: %v", err)
	}
	defer func() { _ = rc.Close() }()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read stored blob: %v", err)
	}
	if string(got) != body {
		t.Errorf("stored %d bytes, want %d (prefix %q)", len(got), len(body), got[:min(8, len(got))])
	}
}
