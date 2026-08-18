// Package media decides how a stored blob (by key) is exposed as a URL based on
// its visibility: public blobs get a stable cacheable URL, protected blobs get a
// short-lived signed URL. For local storage signing is HMAC-SHA256 over the key
// and expiry; for s3 it delegates to native presigned URLs. media also holds the
// write-side normalization that strips signed/legacy URLs back to a bare key.
package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/storage"
)

const avatarTTL = 24 * time.Hour

// Media resolves stored keys to URLs according to visibility.
type Media struct {
	store        storage.Storage
	publicPrefix string        // e.g. "/uploads"
	signKey      []byte        // non-nil only for local; s3 uses store.Presign
	fileTTL      time.Duration // signed-URL TTL for protected files
}

// New constructs a Media. Pass a nil signKey for non-local drivers (s3), which
// routes protected URLs through store.Presign instead of local HMAC.
func New(store storage.Storage, cfg config.StorageConfig, signKey []byte) *Media {
	return &Media{
		store:        store,
		publicPrefix: cfg.LocalBaseURL,
		signKey:      signKey,
		fileTTL:      cfg.SignedURLTTL,
	}
}

// FileTTL is the signed-URL lifetime for protected files.
func (m *Media) FileTTL() time.Duration { return m.fileTTL }

// signedURL returns a time-limited URL for key. Local: publicPrefix/key?exp&sig.
// s3 (signKey nil): a native presigned GET URL.
func (m *Media) signedURL(key string, ttl time.Duration) string {
	if m.signKey == nil {
		u, err := m.store.Presign(context.Background(), key, ttl)
		if err != nil {
			return ""
		}
		return u
	}
	exp := time.Now().Add(ttl).Unix()
	sig := m.sign(key, exp)
	return m.publicPrefix + "/" + key + "?exp=" + strconv.FormatInt(exp, 10) + "&sig=" + sig
}

// sign computes the base64url HMAC of the canonical "key\nexp" message.
func (m *Media) sign(key string, exp int64) string {
	mac := hmac.New(sha256.New, m.signKey)
	mac.Write([]byte(key + "\n" + strconv.FormatInt(exp, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify validates a local signed-URL query (exp + sig) for key in constant time.
// Always false when signKey is nil (s3: signing happens at the storage layer).
func (m *Media) Verify(key string, q url.Values) bool {
	if m.signKey == nil {
		return false
	}
	exp, err := strconv.ParseInt(q.Get("exp"), 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() >= exp {
		return false
	}
	return hmac.Equal([]byte(q.Get("sig")), []byte(m.sign(key, exp)))
}

// ResolveFile returns the URL for a file by visibility: public -> stable URL,
// otherwise a short-lived signed URL.
func (m *Media) ResolveFile(key, visibility string) string {
	if visibility == model.VisibilityPublic {
		return m.store.PublicURL(key)
	}
	return m.signedURL(key, m.fileTTL)
}

// ResolveAvatar returns the URL for a stored avatar value: empty stays empty,
// external http(s) URLs pass through, otherwise a signed URL with avatarTTL.
func (m *Media) ResolveAvatar(stored string) string {
	if stored == "" {
		return ""
	}
	if isExternal(stored) {
		return stored
	}
	return m.signedURL(m.toKey(stored), avatarTTL)
}

// ResolveLogo returns a stable public URL for a stored logo value (logos are
// public). Empty stays empty; external http(s) URLs pass through.
func (m *Media) ResolveLogo(stored string) string {
	if stored == "" {
		return ""
	}
	if isExternal(stored) {
		return stored
	}
	return m.store.PublicURL(m.toKey(stored))
}

// NormalizeStored maps a write-side value (possibly a signed/legacy URL) to the
// canonical stored form: empty stays empty, external URLs pass through, anything
// else is reduced to a bare key. Idempotent.
func (m *Media) NormalizeStored(value string) string {
	if value == "" {
		return ""
	}
	if isExternal(value) {
		return value
	}
	return m.toKey(value)
}

// Open streams a blob by key (passthrough to the underlying store).
func (m *Media) Open(ctx context.Context, key string) (io.ReadSeekCloser, time.Time, error) {
	return m.store.Open(ctx, key)
}

// toKey strips a query string (exp/sig) and the public prefix from value,
// yielding a bare storage key. Tolerates legacy full paths like "/uploads/<key>".
func (m *Media) toKey(value string) string {
	if i := strings.IndexByte(value, '?'); i >= 0 {
		value = value[:i]
	}
	value = strings.TrimPrefix(value, m.publicPrefix+"/")
	return value
}

func isExternal(v string) bool {
	return strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://")
}
