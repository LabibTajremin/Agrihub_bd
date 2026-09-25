package repository

import (
	"context"
	"io"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/domain"
)

// S3 stores blobs in any S3-compatible service (AWS S3, MinIO, Cloudflare R2).
type S3 struct {
	Client *minio.Client
	Bucket string
}

func notFound(err error) error {
	if minio.ToErrorResponse(err).StatusCode == 404 {
		return domain.ErrNotFound
	}
	return err
}

// Put uploads the blob.
func (s S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.Client.PutObject(ctx, s.Bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

// Get streams the blob.
func (s S3) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if _, err := s.Stat(ctx, key); err != nil {
		return nil, err
	}
	return s.Client.GetObject(ctx, s.Bucket, key, minio.GetObjectOptions{})
}

// Stat reports the blob size.
func (s S3) Stat(ctx context.Context, key string) (domain.ObjectInfo, error) {
	info, err := s.Client.StatObject(ctx, s.Bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return domain.ObjectInfo{}, notFound(err)
	}
	return domain.ObjectInfo{Size: info.Size}, nil
}

// Delete removes the blob.
func (s S3) Delete(ctx context.Context, key string) error {
	return s.Client.RemoveObject(ctx, s.Bucket, key, minio.RemoveObjectOptions{})
}

// PresignPut returns a presigned upload URL.
func (s S3) PresignPut(ctx context.Context, key, _ string, ttl time.Duration) (string, error) {
	u, err := s.Client.PresignedPutObject(ctx, s.Bucket, key, ttl)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// PresignGet returns a presigned download URL.
func (s S3) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.Client.PresignedGetObject(ctx, s.Bucket, key, ttl, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
