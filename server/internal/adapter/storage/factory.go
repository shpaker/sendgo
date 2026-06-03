package storage

import (
	"context"
	"log/slog"

	"github.com/sendgo/sendgo/server/internal/config"
	"github.com/sendgo/sendgo/server/internal/port"
)

// New selects a BlobStorage based on config:
//   - if --s3-bucket is set → S3 (compatible with Yandex Object Storage / MinIO / AWS),
//   - otherwise → FS.
//
// Priority logic mirrors server/storage/index.js (Node) 1:1.
func New(ctx context.Context, cfg *config.CLI, lg *slog.Logger) (port.BlobStorage, error) {
	if cfg.S3Bucket != "" {
		lg.Info("storage backend: s3",
			"bucket", cfg.S3Bucket,
			"endpoint", cfg.S3Endpoint,
			"region", cfg.S3Region,
		)
		return NewS3Storage(ctx, cfg)
	}
	lg.Info("storage backend: fs", "dir", cfg.FileDir)
	return NewFSStorage(cfg.FileDir)
}
