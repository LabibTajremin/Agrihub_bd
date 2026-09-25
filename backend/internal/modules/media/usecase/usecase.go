// Package usecase implements the media interactors: upload tickets, upload
// completion with checksum verification, and download links.
package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/tx"
)

// TopicObjectUploaded is published when an upload is verified.
const TopicObjectUploaded = "media.object.uploaded"

// Policy bounds uploads (from config).
type Policy struct {
	AllowedTypes []string
	MaxBytes     int64
	PresignTTL   time.Duration
}

// Deps are shared by the media use cases.
type Deps struct {
	Objects domain.Objects
	Storage domain.Storage
	Tx      tx.Manager
	Clock   clock.Clock
	IDs     idgen.Generator
	Events  eventbus.Publisher
	Factory eventbus.Factory
	Policy  Policy
}

// TicketInput declares the file about to be uploaded.
type TicketInput struct {
	ContentType string
	SizeBytes   int64
	Checksum    string
}

// Ticket authorises one direct upload.
type Ticket struct {
	Object    domain.Object
	UploadURL string
	ExpiresAt time.Time
}

// Key is the storage key of an object: m/<owner>/<id>.
func Key(ownerID, id string) string { return "m/" + ownerID + "/" + id }

// IDFromKey extracts the object ID from a storage key.
func IDFromKey(key string) string { return key[strings.LastIndexByte(key, '/')+1:] }

// CreateTicket validates the declared file and returns a presigned upload URL.
type CreateTicket struct{ Deps }

// Execute creates a pending object and its upload URL.
func (uc CreateTicket) Execute(ctx context.Context, in TicketInput) (Ticket, error) {
	p, err := authn.Require(ctx, authz.MediaUpload)
	if err != nil {
		return Ticket{}, err
	}
	if !slices.Contains(uc.Policy.AllowedTypes, in.ContentType) {
		return Ticket{}, domain.ErrUnsupportedType
	}
	if in.SizeBytes <= 0 || in.SizeBytes > uc.Policy.MaxBytes {
		return Ticket{}, domain.ErrTooLarge
	}
	now := uc.Clock.Now()
	id := uc.IDs.New()
	o := domain.Object{ID: id, OwnerID: p.Subject, Key: Key(p.Subject, id), ContentType: in.ContentType, SizeBytes: in.SizeBytes,
		Checksum: strings.ToLower(in.Checksum), Status: domain.StatusPending, CreatedAt: now}
	if err := uc.Objects.Create(ctx, o); err != nil {
		return Ticket{}, err
	}
	u, err := uc.Storage.PresignPut(ctx, o.Key, o.ContentType, uc.Policy.PresignTTL)
	if err != nil {
		return Ticket{}, err
	}
	return Ticket{Object: o, UploadURL: u, ExpiresAt: now.Add(uc.Policy.PresignTTL)}, nil
}

// Complete verifies an uploaded blob against its declared size and SHA-256,
// computes the perceptual hash of images, and marks the object uploaded.
// A mismatching blob is deleted.
type Complete struct{ Deps }

// Execute verifies object id (idempotent once uploaded).
func (uc Complete) Execute(ctx context.Context, id string) (domain.Object, error) {
	p, err := authn.Require(ctx, authz.MediaUpload)
	if err != nil {
		return domain.Object{}, err
	}
	o, err := uc.Objects.Get(ctx, id)
	if err != nil {
		return domain.Object{}, err
	}
	if o.OwnerID != p.Subject {
		return domain.Object{}, authz.ErrForbidden
	}
	if o.Status == domain.StatusUploaded {
		return o, nil
	}
	data, err := uc.read(ctx, o)
	if err != nil {
		return domain.Object{}, err
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != o.SizeBytes || hex.EncodeToString(sum[:]) != o.Checksum {
		return domain.Object{}, errors.Join(domain.ErrChecksumMismatch, uc.Storage.Delete(ctx, o.Key))
	}
	if strings.HasPrefix(o.ContentType, "image/") {
		if o.PHash, err = domain.DHash(bytes.NewReader(data)); err != nil {
			return domain.Object{}, domain.ErrUnsupportedType.Wrap(err)
		}
	}
	now := uc.Clock.Now()
	o.Status, o.UploadedAt = domain.StatusUploaded, &now
	err = uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.Objects.MarkUploaded(ctx, o.ID, o.PHash, now); err != nil {
			return err
		}
		e, err := uc.Factory.New(ctx, TopicObjectUploaded, p.Subject, map[string]string{"media_id": o.ID, "content_type": o.ContentType})
		if err == nil {
			err = uc.Events.Publish(ctx, e)
		}
		return err
	})
	return o, err
}

func (uc Complete) read(ctx context.Context, o domain.Object) ([]byte, error) {
	rc, err := uc.Storage.Get(ctx, o.Key)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrNotUploaded
	}
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, uc.Policy.MaxBytes+1))
}

// Download is an object with a time-limited download URL.
type Download struct {
	Object domain.Object
	URL    string
}

// GetObject returns metadata and a download link (owner, or reviewers with scan:read_any).
type GetObject struct{ Deps }

// Execute loads object id.
func (uc GetObject) Execute(ctx context.Context, id string) (Download, error) {
	p, err := authn.Require(ctx, authz.MediaRead)
	if err != nil {
		return Download{}, err
	}
	o, err := uc.Objects.Get(ctx, id)
	if err != nil {
		return Download{}, err
	}
	if err := authz.Owned(p.Role, p.Subject, o.OwnerID, authz.MediaRead, authz.ScanReadAny); err != nil {
		return Download{}, err
	}
	if o.Status != domain.StatusUploaded {
		return Download{}, domain.ErrNotUploaded
	}
	u, err := uc.Storage.PresignGet(ctx, o.Key, uc.Policy.PresignTTL)
	return Download{Object: o, URL: u}, err
}

// Verifier checks locally signed blob URLs (local-disk backend only).
type Verifier interface {
	Verify(method, key, exp, sig string) error
}

// BlobPut receives a direct upload for the local-disk backend.
type BlobPut struct {
	Deps
	Verifier Verifier
}

// Execute verifies the signature and stores the body.
func (uc BlobPut) Execute(ctx context.Context, key, exp, sig, contentType string, body io.Reader) error {
	if err := uc.Verifier.Verify("PUT", key, exp, sig); err != nil {
		return err
	}
	o, err := uc.Objects.Get(ctx, IDFromKey(key))
	if err != nil {
		return err
	}
	if o.Key != key || o.ContentType != contentType {
		return domain.ErrTicketInvalid
	}
	return uc.Storage.Put(ctx, key, body, o.SizeBytes, contentType)
}

// BlobGet serves a signed download for the local-disk backend.
type BlobGet struct {
	Deps
	Verifier Verifier
}

// Execute verifies the signature and opens the blob.
func (uc BlobGet) Execute(ctx context.Context, key, exp, sig string) (io.ReadCloser, error) {
	if err := uc.Verifier.Verify("GET", key, exp, sig); err != nil {
		return nil, err
	}
	return uc.Storage.Get(ctx, key)
}
