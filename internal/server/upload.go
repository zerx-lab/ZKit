package server

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/storage"
)

const maxUploadBytes = 20 << 20 // 20 MiB

// allowedExt is the whitelist of upload file extensions (lowercased, with dot).
var allowedExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".svg": true, ".pdf": true, ".txt": true, ".csv": true, ".json": true,
	".zip": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true, ".md": true, ".mp4": true, ".mp3": true,
}

// uploadHandler accepts a single multipart "file" from any authenticated user,
// stores it, records its metadata, and returns the file JSON.
func uploadHandler(issuer *auth.Issuer, store storage.Storage, m *media.Media, db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		claims, err := issuer.ParseAccess(raw)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
		if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
			http.Error(w, "file too large or invalid form", http.StatusBadRequest)
			return
		}

		f, hdr, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file field", http.StatusBadRequest)
			return
		}
		defer func() { _ = f.Close() }()

		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		if ext != "" && !allowedExt[ext] {
			http.Error(w, "unsupported file type", http.StatusBadRequest)
			return
		}

		visibility := r.FormValue("visibility")
		switch visibility {
		case model.VisibilityPublic, model.VisibilityAuthenticated, model.VisibilityPrivate:
		default:
			visibility = model.VisibilityPrivate
		}
		if visibility == model.VisibilityPublic && !slices.Contains(claims.Roles, model.RoleAdmin) {
			http.Error(w, "只有管理员可发布匿名可访问文件", http.StatusForbidden)
			return
		}

		key := time.Now().Format("2006/01") + "/" + uuid.NewString() + ext
		contentType := hdr.Header.Get("Content-Type")

		if err := store.Save(r.Context(), key, f, hdr.Size, contentType); err != nil {
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}

		rec := model.File{
			Name:        hdr.Filename,
			Key:         key,
			Size:        hdr.Size,
			ContentType: contentType,
			Visibility:  visibility,
			UploadedBy:  claims.UserID,
		}
		if err := gorm.G[model.File](db).Create(context.Background(), &rec); err != nil {
			http.Error(w, "record failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          rec.ID,
			"name":        rec.Name,
			"key":         rec.Key,
			"url":         m.ResolveFile(rec.Key, rec.Visibility),
			"size":        rec.Size,
			"contentType": rec.ContentType,
			"visibility":  rec.Visibility,
		})
	}
}
