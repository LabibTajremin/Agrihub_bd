// Package media is the media module (future media-service): upload tickets,
// verified blobs in object storage and perceptual hashes of images.
package media

import (
	"context"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/config"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/kernel"
)

// Deps are the module's dependencies.
type Deps struct {
	Kernel  kernel.Kernel
	Storage domain.Storage
	// Local is set when Storage is the local-disk backend; it enables the
	// signed /v1/media/blob routes that stand in for S3 presigned URLs.
	Local  *repository.LocalDisk
	Policy usecase.Policy
}

// Module is the assembled media module.
type Module struct {
	handlers transport.Handlers
	objects  domain.Objects
	storage  domain.Storage
}

// New wires the module.
func New(d Deps) (*Module, error) {
	k := d.Kernel
	objects := repository.Objects{DB: k.DB}
	ud := usecase.Deps{Objects: objects, Storage: d.Storage, Tx: k.DB, Clock: k.Clock, IDs: k.IDs, Events: k.Events, Factory: k.Factory(), Policy: d.Policy}
	h := transport.Handlers{Ticket: usecase.CreateTicket{Deps: ud}, Complete: usecase.Complete{Deps: ud}, Get: usecase.GetObject{Deps: ud}, V: k.Validator}
	if d.Local != nil {
		h.BlobPut = &usecase.BlobPut{Deps: ud, Verifier: d.Local}
		h.BlobGet = &usecase.BlobGet{Deps: ud, Verifier: d.Local}
	}
	return &Module{handlers: h, objects: objects, storage: d.Storage}, nil
}

// NewStorage builds the configured backend. The *LocalDisk is non-nil only for
// the local backend.
func NewStorage(cfg config.Storage, baseURL string, c clock.Clock) (domain.Storage, *repository.LocalDisk, error) {
	if cfg.Backend == "local" {
		l := &repository.LocalDisk{Root: cfg.LocalDir, BaseURL: baseURL, Key: []byte(cfg.LocalSigningKey), Clock: c}
		return l, l, nil
	}
	client, err := minio.New(cfg.S3Endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""), Secure: cfg.S3UseSSL, Region: cfg.S3Region,
	})
	if err != nil {
		return nil, nil, err
	}
	return repository.S3{Client: client, Bucket: cfg.S3Bucket}, nil, nil
}

// Routes returns the module's HTTP routes.
func (m *Module) Routes() []httpx.Route { return m.handlers.Routes() }

// Errors is the module's error catalogue.
func Errors() []*errs.Error { return domain.Errors() }

// ObjectView is the published media representation.
type ObjectView struct {
	ID          string
	OwnerID     string
	ContentType string
	PHash       uint64
	Uploaded    bool
	UploadedAt  *time.Time
}

// Object returns the published view of object id (callers authorize).
func (m *Module) Object(ctx context.Context, id string) (ObjectView, error) {
	o, err := m.objects.Get(ctx, id)
	if err != nil {
		return ObjectView{}, err
	}
	return ObjectView{ID: o.ID, OwnerID: o.OwnerID, ContentType: o.ContentType, PHash: o.PHash,
		Uploaded: o.Status == domain.StatusUploaded, UploadedAt: o.UploadedAt}, nil
}

// Open streams the bytes of an uploaded object (callers authorize).
func (m *Module) Open(ctx context.Context, id string) (io.ReadCloser, error) {
	o, err := m.objects.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return m.storage.Get(ctx, o.Key)
}
