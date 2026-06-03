package storage

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/sendgo/sendgo/server/internal/config"
	"github.com/sendgo/sendgo/server/internal/domain"
)

// S3Storage is the S3-compatible store. Used when CLI.S3Bucket != "".
// The primary target backend is Yandex Object Storage
// (`storage.yandexcloud.net`), but the config is generic: the same adapter
// works against AWS, MinIO, and so on.
//
// feature/s3/manager is marked deprecated in favor of
// feature/s3/transfermanager, but the new transfermanager currently demands
// more explicit buffer configuration and is not end-to-end tested against
// Yandex/MinIO — we stick with the stable manager.
//
//nolint:staticcheck // SA1019: the deprecated manager is still supported by the AWS SDK
type S3Storage struct {
	bucket   string
	client   *s3.Client
	uploader *manager.Uploader
}

// NewS3Storage assembles an aws-sdk-go-v2 client, honoring an optional
// custom endpoint (Yandex/MinIO) and path-style addressing (required for
// MinIO).
func NewS3Storage(ctx context.Context, cfg *config.CLI) (*S3Storage, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.S3Region),
	)
	if err != nil {
		return nil, fmt.Errorf("s3: load aws config: %w", err)
	}

	opts := []func(*s3.Options){}
	if cfg.S3Endpoint != "" {
		ep := cfg.S3Endpoint
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(ep)
		})
	}
	if cfg.S3UsePathStyle {
		opts = append(opts, func(o *s3.Options) { o.UsePathStyle = true })
	}

	client := s3.NewFromConfig(awsCfg, opts...)
	up := manager.NewUploader(client) //nolint:staticcheck // SA1019: see comment on S3Storage
	return &S3Storage{bucket: cfg.S3Bucket, client: client, uploader: up}, nil
}

// Put streams r into S3 via s3manager.Uploader (multipart under the hood for
// large objects). expireSeconds is unused (S3 TTL is configured via a
// lifecycle policy at the bucket level).
func (s *S3Storage) Put(ctx context.Context, key string, r io.Reader, expireSeconds int) error {
	_, err := s.uploader.Upload(ctx, &s3.PutObjectInput{ //nolint:staticcheck // SA1019: see comment on S3Storage
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	})
	if err != nil {
		return fmt.Errorf("s3 put %s/%s: %w", s.bucket, key, err)
	}
	return nil
}

func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("s3 get %s/%s: %w", s.bucket, key, err)
	}
	return out.Body, nil
}

func (s *S3Storage) Head(ctx context.Context, key string) (int64, bool, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("s3 head %s/%s: %w", s.bucket, key, err)
	}
	return aws.ToInt64(out.ContentLength), true, nil
}

func (s *S3Storage) Length(ctx context.Context, key string) (int64, error) {
	size, ok, err := s.Head(ctx, key)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, domain.ErrNotFound
	}
	return size, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil && !isS3NotFound(err) {
		return fmt.Errorf("s3 delete %s/%s: %w", s.bucket, key, err)
	}
	return nil
}

func (s *S3Storage) Ping(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		return fmt.Errorf("s3 ping: %w", err)
	}
	return nil
}

// isS3NotFound: S3 returns NoSuchKey or NotFound depending on the operation.
func isS3NotFound(err error) bool {
	var noKey *types.NoSuchKey
	if errors.As(err, &noKey) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}
