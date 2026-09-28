// Package domain holds the media model: stored objects, the storage port and
// the perceptual hash used to recognise a re-scanned leaf.
package domain

import (
	"context"
	"io"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// Object statuses.
const (
	StatusPending  = "pending"
	StatusUploaded = "uploaded"
)

// Object is an uploaded blob's metadata.
type Object struct {
	ID          string
	OwnerID     string
	Key         string
	ContentType string
	SizeBytes   int64
	Checksum    string // hex SHA-256 declared by the client
	PHash       uint64 // dHash for images, 0 otherwise
	Status      string
	CreatedAt   time.Time
	UploadedAt  *time.Time
}

// ObjectInfo is what the storage backend reports about a blob.
type ObjectInfo struct {
	Size int64
}

// Errors.
var (
	ErrNotFound         = errs.NotFound("media.not_found")
	ErrUnsupportedType  = errs.Validation("media.unsupported_type")
	ErrTooLarge         = errs.Validation("media.too_large")
	ErrChecksumMismatch = errs.Validation("media.checksum_mismatch")
	ErrTicketInvalid    = errs.Unauthorized("media.ticket_invalid")
	ErrNotUploaded      = errs.Conflict("media.not_uploaded")
)

// Errors lists the media error catalogue.
func Errors() []*errs.Error {
	return []*errs.Error{ErrNotFound, ErrUnsupportedType, ErrTooLarge, ErrChecksumMismatch, ErrTicketInvalid, ErrNotUploaded}
}

// Storage is the blob storage port (S3-compatible or local disk).
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Stat returns ErrNotFound when the blob is absent.
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (string, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// Objects is the metadata repository port.
type Objects interface {
	Create(ctx context.Context, o Object) error
	Get(ctx context.Context, id string) (Object, error)
	MarkUploaded(ctx context.Context, id string, phash uint64, at time.Time) error
}
