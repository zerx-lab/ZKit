package service

import (
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/media"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/storage"
)

// FileService implements zerxv1connect.FileServiceHandler.
type FileService struct {
	db    *gorm.DB
	store storage.Storage
	media *media.Media
}

var _ zerxv1connect.FileServiceHandler = (*FileService)(nil)

// NewFileService constructs the file handler.
func NewFileService(db *gorm.DB, store storage.Storage, m *media.Media) *FileService {
	return &FileService{db: db, store: store, media: m}
}

func (s *FileService) ListFiles(ctx context.Context, req *connect.Request[zerxv1.ListFilesRequest]) (*connect.Response[zerxv1.ListFilesResponse], error) {
	_, ps, offset := normalizePage(req.Msg.GetPage().GetPage(), req.Msg.GetPage().GetPageSize())

	// Apply the keyword filter and a visibility filter for non-admin callers
	// (they see public/authenticated files plus their own).
	apply := func() gorm.ChainInterface[model.File] {
		q := gorm.G[model.File](s.db).Where("1 = 1")
		if kw := req.Msg.GetKeyword(); kw != "" {
			q = q.Where("name LIKE ?", "%"+kw+"%")
		}
		if claims, ok := auth.ClaimsFromContext(ctx); ok && !slices.Contains(claims.Roles, model.RoleAdmin) {
			q = q.Where("visibility IN ? OR uploaded_by = ?",
				[]string{model.VisibilityPublic, model.VisibilityAuthenticated}, claims.UserID)
		}
		return q
	}
	total, err := apply().Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	files, err := apply().Order("id DESC").Limit(ps).Offset(offset).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.ListFilesResponse{Files: protoFiles(files, s.media), Total: total}), nil
}

func protoFiles(files []model.File, m *media.Media) []*zerxv1.File {
	out := make([]*zerxv1.File, 0, len(files))
	for i := range files {
		out = append(out, toProtoFile(files[i], m))
	}

	return out
}

func (s *FileService) DeleteFile(ctx context.Context, req *connect.Request[zerxv1.DeleteFileRequest]) (*connect.Response[zerxv1.DeleteFileResponse], error) {
	f, err := gorm.G[model.File](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("file not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if claims, ok := auth.ClaimsFromContext(ctx); ok &&
		!slices.Contains(claims.Roles, model.RoleAdmin) && f.UploadedBy != claims.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("无权删除该文件"))
	}

	if _, err := gorm.G[model.File](s.db).Where("id = ?", f.ID).Delete(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.store.Delete(ctx, f.Key); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&zerxv1.DeleteFileResponse{}), nil
}
