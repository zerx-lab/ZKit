// Package storage abstracts object persistence behind a small interface with a
// local-disk and an S3-compatible (minio) implementation.
package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/zerx-lab/zkit/internal/config"
)

// Storage saves, deletes, and reads blobs by key.
type Storage interface {
	Save(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Delete(ctx context.Context, key string) error
	// PublicURL returns a stable, cacheable URL for a publicly served blob,
	// or "" when no public base URL is configured.
	PublicURL(key string) string
	// Open streams a blob for reading along with its modification time.
	Open(ctx context.Context, key string) (io.ReadSeekCloser, time.Time, error)
	// Presign returns a time-limited URL for a protected blob (s3 presigned GET);
	// local returns "" with a nil error (local uses media-layer HMAC instead).
	Presign(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// New constructs a Storage from configuration; driver is "local" or "s3".
func New(cfg config.StorageConfig) (Storage, error) {
	switch cfg.Driver {
	case "local":
		return &Local{dir: cfg.LocalDir, baseURL: cfg.LocalBaseURL}, nil
	case "s3":
		client, err := minio.New(cfg.S3Endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
			Secure: cfg.S3Secure,
			Region: cfg.S3Region,
		})
		if err != nil {
			return nil, fmt.Errorf("init s3 client: %w", err)
		}

		return &S3{client: client, bucket: cfg.S3Bucket, publicBaseURL: cfg.S3PublicURL}, nil
	default:
		return nil, fmt.Errorf("unsupported storage driver %q (want local|s3)", cfg.Driver)
	}
}
