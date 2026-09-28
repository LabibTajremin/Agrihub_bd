// Package transport exposes media over HTTP.
package transport

import (
	"io"
	"net/http"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// TicketRequest declares an upload.
type TicketRequest struct {
	ContentType string `json:"content_type" validate:"required,max=100"`
	SizeBytes   int64  `json:"size_bytes" validate:"gt=0"`
	Checksum    string `json:"checksum_sha256" validate:"required,len=64,hexadecimal"`
}

// TicketResponse tells the client where to PUT the bytes.
type TicketResponse struct {
	MediaID   string            `json:"media_id"`
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// Object is media metadata.
type Object struct {
	ID          string     `json:"id"`
	ContentType string     `json:"content_type"`
	SizeBytes   int64      `json:"size_bytes"`
	Checksum    string     `json:"checksum_sha256"`
	PHash       string     `json:"phash"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UploadedAt  *time.Time `json:"uploaded_at,omitempty"`
	DownloadURL string     `json:"download_url,omitempty"`
}

// Handlers groups the media endpoints.
type Handlers struct {
	Ticket   usecase.CreateTicket
	Complete usecase.Complete
	Get      usecase.GetObject
	BlobPut  *usecase.BlobPut // nil unless the local-disk backend is active
	BlobGet  *usecase.BlobGet
	V        *validator.Validator
}

// Routes declares the endpoints.
func (h Handlers) Routes() []httpx.Route {
	rs := []httpx.Route{
		{Method: http.MethodPost, Path: "/v1/media/tickets", Permission: string(authz.MediaUpload), Tag: "media", Summary: "Request an upload ticket",
			Request: TicketRequest{}, Response: TicketResponse{}, Status: http.StatusCreated, Handler: h.ticket},
		{Method: http.MethodPost, Path: "/v1/media/{id}/complete", Permission: string(authz.MediaUpload), Tag: "media", Summary: "Verify an upload",
			Response: Object{}, Handler: h.complete},
		{Method: http.MethodGet, Path: "/v1/media/{id}", Permission: string(authz.MediaRead), Tag: "media", Summary: "Media metadata and download link",
			Response: Object{}, Handler: h.get},
	}
	if h.BlobPut != nil {
		rs = append(rs,
			httpx.Route{Method: http.MethodPut, Path: "/v1/media/blob/{key...}", Tag: "media", Summary: "Signed direct upload (local backend)",
				MaxBodyBytes: h.Ticket.Policy.MaxBytes + 1, Handler: h.blobPut},
			httpx.Route{Method: http.MethodGet, Path: "/v1/media/blob/{key...}", Tag: "media", Summary: "Signed download (local backend)", Handler: h.blobGet},
		)
	}
	return rs
}

// ToObject renders metadata.
func ToObject(o domain.Object, url string) Object {
	return Object{ID: o.ID, ContentType: o.ContentType, SizeBytes: o.SizeBytes, Checksum: o.Checksum,
		PHash: formatHash(o.PHash), Status: o.Status, CreatedAt: o.CreatedAt, UploadedAt: o.UploadedAt, DownloadURL: url}
}

func formatHash(h uint64) string {
	const digits = "0123456789abcdef"
	b := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		b[i] = digits[h&0xf]
		h >>= 4
	}
	return string(b)
}

func (h Handlers) ticket(w http.ResponseWriter, r *http.Request) error {
	var in TicketRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	t, err := h.Ticket.Execute(r.Context(), usecase.TicketInput{ContentType: in.ContentType, SizeBytes: in.SizeBytes, Checksum: in.Checksum})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, TicketResponse{MediaID: t.Object.ID, UploadURL: t.UploadURL, Method: http.MethodPut,
		Headers: map[string]string{"Content-Type": t.Object.ContentType}, ExpiresAt: t.ExpiresAt})
}

func (h Handlers) complete(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	o, err := h.Complete.Execute(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, ToObject(o, ""))
}

func (h Handlers) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	d, err := h.Get.Execute(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, ToObject(d.Object, d.URL))
}

func (h Handlers) blobPut(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	if err := h.BlobPut.Execute(r.Context(), r.PathValue("key"), q.Get("exp"), q.Get("sig"), r.Header.Get("Content-Type"), r.Body); err != nil {
		return err
	}
	return httpx.NoContent(w)
}

func (h Handlers) blobGet(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	rc, err := h.BlobGet.Execute(r.Context(), r.PathValue("key"), q.Get("exp"), q.Get("sig"))
	if err != nil {
		return err
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	_, err = io.Copy(w, rc)
	return err
}
