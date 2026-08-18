package server

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/model"
)

// mediaHandler serves locally stored blobs with visibility-aware authorization.
// public blobs are served to anyone (cacheable); protected blobs require either
// a valid media signature (a capability token) or a Bearer access token whose
// claims satisfy the visibility (authenticated: any user; private: owner/admin).
func mediaHandler(issuer *auth.Issuer, m *media.Media, db *gorm.DB, prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		key := strings.TrimPrefix(r.URL.Path, prefix+"/")
		if key == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		f, err := gorm.G[model.File](db).Where("key = ?", key).First(r.Context())
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if !authorizeMedia(issuer, m, f, r, w) {
			return
		}

		// Security headers for all responses: SVG/HTML uploads can carry script;
		// nosniff + a sandboxing CSP neutralize inline execution in the browser.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "sandbox")

		rc, modtime, err := m.Open(r.Context(), key)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer func() { _ = rc.Close() }()

		w.Header().Set("Content-Type", f.ContentType)
		http.ServeContent(w, r, f.Name, modtime, rc)
	}
}

// authorizeMedia decides whether the request may read f, writing the appropriate
// status + cache header on success and the rejection status on failure. It
// returns true only when the caller is authorized.
func authorizeMedia(issuer *auth.Issuer, m *media.Media, f model.File, r *http.Request, w http.ResponseWriter) bool {
	if f.Visibility == model.VisibilityPublic {
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
		return true
	}

	protectedCache := func() {
		w.Header().Set("Cache-Control", "private, max-age="+strconv.Itoa(int(m.FileTTL().Seconds())))
	}

	// A valid media signature is a capability token: it satisfies both
	// authenticated and private visibility.
	if m.Verify(f.Key, r.URL.Query()) {
		protectedCache()
		return true
	}

	raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	claims, err := issuer.ParseAccess(raw)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}

	if f.Visibility == model.VisibilityAuthenticated {
		protectedCache()
		return true
	}

	// private: owner or admin only.
	if claims.UserID == f.UploadedBy || slices.Contains(claims.Roles, model.RoleAdmin) {
		protectedCache()
		return true
	}

	http.Error(w, "forbidden", http.StatusForbidden)
	return false
}
