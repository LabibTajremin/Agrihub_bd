// Package repository holds the media adapters: Postgres metadata and the
// local-disk and S3 storage backends.
package repository

import (
	"context"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
)

// Objects implements domain.Objects.
type Objects struct{ DB *database.DB }

// Create inserts pending object metadata.
func (r Objects) Create(ctx context.Context, o domain.Object) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO media_objects (id, owner_id, key, content_type, size_bytes, checksum, phash, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, o.ID, o.OwnerID, o.Key, o.ContentType, o.SizeBytes, o.Checksum, int64(o.PHash), o.Status, o.CreatedAt) //nolint:gosec // bit pattern stored as-is
	return err
}

// Get loads object metadata.
func (r Objects) Get(ctx context.Context, id string) (domain.Object, error) {
	var o domain.Object
	var phash int64
	err := r.DB.Q(ctx).QueryRow(ctx, `SELECT id, owner_id, key, content_type, size_bytes, checksum, phash, status, created_at, uploaded_at
		FROM media_objects WHERE id = $1`, id).
		Scan(&o.ID, &o.OwnerID, &o.Key, &o.ContentType, &o.SizeBytes, &o.Checksum, &phash, &o.Status, &o.CreatedAt, &o.UploadedAt)
	o.PHash, o.CreatedAt = uint64(phash), o.CreatedAt.UTC() //nolint:gosec // bit pattern stored as-is
	return o, database.NotFound(err, domain.ErrNotFound)
}

// MarkUploaded records a verified upload.
func (r Objects) MarkUploaded(ctx context.Context, id string, phash uint64, at time.Time) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `UPDATE media_objects SET status = 'uploaded', phash = $2, uploaded_at = $3 WHERE id = $1`, id, int64(phash), at) //nolint:gosec // bit pattern
	return err
}
