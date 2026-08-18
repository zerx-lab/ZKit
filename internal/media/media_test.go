package media

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/storage"
)

func newLocal(t *testing.T) *Media {
	t.Helper()
	store, err := storage.New(config.StorageConfig{Driver: "local", LocalDir: t.TempDir(), LocalBaseURL: "/uploads"})
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	return New(store, config.StorageConfig{LocalBaseURL: "/uploads", SignedURLTTL: time.Hour}, []byte("test-sign-key"))
}

func TestVerify(t *testing.T) {
	m := newLocal(t)
	key := "2026/06/x.png"
	signed := m.signedURL(key, time.Hour)

	u, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !m.Verify(key, u.Query()) {
		t.Error("valid signature should verify")
	}

	// Tampered sig.
	bad := u.Query()
	bad.Set("sig", "deadbeef")
	if m.Verify(key, bad) {
		t.Error("tampered sig must not verify")
	}

	// Expired exp.
	expired := m.signedURL(key, -time.Hour)
	eu, _ := url.Parse(expired)
	if m.Verify(key, eu.Query()) {
		t.Error("expired exp must not verify")
	}
}

func TestNormalizeIdempotent(t *testing.T) {
	m := newLocal(t)
	key := "2026/06/x.png"
	signed := m.signedURL(key, time.Hour)

	if got := m.NormalizeStored(signed); got != key {
		t.Errorf("NormalizeStored(signed) = %q, want %q", got, key)
	}
	if got := m.NormalizeStored(key); got != key {
		t.Errorf("NormalizeStored(key) = %q, want %q", got, key)
	}
	// Legacy full path.
	if got := m.NormalizeStored("/uploads/" + key); got != key {
		t.Errorf("NormalizeStored(legacy) = %q, want %q", got, key)
	}
}

func TestResolveAvatarExternal(t *testing.T) {
	m := newLocal(t)
	ext := "https://x.example/a.png"
	if got := m.ResolveAvatar(ext); got != ext {
		t.Errorf("ResolveAvatar(external) = %q, want passthrough", got)
	}
	if got := m.ResolveAvatar(""); got != "" {
		t.Errorf("ResolveAvatar(empty) = %q, want empty", got)
	}
	signed := m.ResolveAvatar("2026/06/x.png")
	if !strings.HasPrefix(signed, "/uploads/2026/06/x.png?exp=") {
		t.Errorf("ResolveAvatar(key) = %q, want signed URL", signed)
	}
}
