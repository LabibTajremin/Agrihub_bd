// Package transport exposes diagnosis over HTTP.
package transport

import (
	"net/http"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// CreateScanRequest asks for a diagnosis of an uploaded photo.
type CreateScanRequest struct {
	MediaID        string    `json:"media_id" validate:"required,uuid"`
	CropCode       string    `json:"crop_code" validate:"required,max=40"`
	FieldID        string    `json:"field_id" validate:"omitempty,uuid"`
	IdempotencyKey string    `json:"idempotency_key" validate:"max=64"`
	CapturedAt     time.Time `json:"captured_at"`
	Lang           string    `json:"lang" validate:"omitempty,max=5"`
}

// Plan is one treatment variant (steps are dictionary keys).
type Plan struct {
	Variant   string   `json:"variant"`
	Steps     []string `json:"steps"`
	SafetyKey string   `json:"safety_key,omitempty"`
}

// Diagnosis is the analysed result.
type Diagnosis struct {
	DiseaseCode    string  `json:"disease_code"`
	NameKey        string  `json:"name_key"`
	DescriptionKey string  `json:"description_key"`
	Confidence     float64 `json:"confidence"`
	Severity       string  `json:"severity"`
	Healthy        bool    `json:"healthy"`
	ModelVersion   string  `json:"model_version"`
	Plans          []Plan  `json:"plans"`
}

// Scan is the scan representation.
type Scan struct {
	ID         string     `json:"id"`
	MediaID    string     `json:"media_id"`
	FieldID    string     `json:"field_id,omitempty"`
	CropCode   string     `json:"crop_code"`
	Status     string     `json:"status"`
	CapturedAt time.Time  `json:"captured_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Note       string     `json:"note"`
	Saved      bool       `json:"saved"`
	Diagnosis  *Diagnosis `json:"diagnosis,omitempty"`
}

// ScanList is a page of scans.
type ScanList struct {
	Scans      []Scan     `json:"scans"`
	NextBefore *time.Time `json:"next_before,omitempty"`
}

// ListQuery filters scans.
type ListQuery struct {
	Saved   bool      `query:"saved"`
	OwnerID string    `query:"owner_id"`
	Limit   int       `query:"limit"`
	Before  time.Time `query:"before"`
}

// AnnotateRequest edits note / saved.
type AnnotateRequest struct {
	Note  *string `json:"note" validate:"omitempty,max=500"`
	Saved *bool   `json:"saved"`
}

// SyncOperation is one queued offline operation.
type SyncOperation struct {
	IdempotencyKey string             `json:"idempotency_key" validate:"required,max=64"`
	Seq            int64              `json:"seq"`
	Kind           string             `json:"kind" validate:"oneof=create_scan annotate_scan"`
	Create         *CreateScanRequest `json:"create,omitempty"`
	ScanID         string             `json:"scan_id" validate:"omitempty,uuid"`
	Annotation     *AnnotateRequest   `json:"annotation,omitempty"`
}

// SyncRequest uploads an offline queue.
type SyncRequest struct {
	Operations []SyncOperation `json:"operations" validate:"required,min=1,max=200,dive"`
}

// SyncResult reports one operation.
type SyncResult struct {
	IdempotencyKey string `json:"idempotency_key"`
	Outcome        string `json:"outcome"`
	ScanID         string `json:"scan_id,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
}

// SyncResponse lists results in processing (seq) order.
type SyncResponse struct {
	Results []SyncResult `json:"results"`
}

// Handlers groups the diagnosis endpoints.
type Handlers struct {
	Create   usecase.CreateScan
	Retry    usecase.Retry
	Get      usecase.GetScan
	List     usecase.ListScans
	Annotate usecase.Annotate
	Sync     usecase.Sync
	V        *validator.Validator
}

// Routes declares the endpoints.
func (h Handlers) Routes() []httpx.Route {
	return []httpx.Route{
		{Method: http.MethodPost, Path: "/v1/scans", Permission: string(authz.ScanCreate), Tag: "diagnosis", Summary: "Diagnose an uploaded leaf photo (idempotent)",
			Request: CreateScanRequest{}, Response: Scan{}, Status: http.StatusCreated, Handler: h.create},
		{Method: http.MethodGet, Path: "/v1/scans", Permission: string(authz.ScanRead), Tag: "diagnosis", Summary: "Scan history / saved log",
			Query: ListQuery{}, Response: ScanList{}, Handler: h.list},
		{Method: http.MethodGet, Path: "/v1/scans/{id}", Permission: string(authz.ScanRead), Tag: "diagnosis", Summary: "One scan",
			Response: Scan{}, Handler: h.get},
		{Method: http.MethodPatch, Path: "/v1/scans/{id}", Permission: string(authz.ScanCreate), Tag: "diagnosis", Summary: "Annotate (note, save to log)",
			Request: AnnotateRequest{}, Response: Scan{}, Handler: h.annotate},
		{Method: http.MethodPost, Path: "/v1/scans/{id}/retry", Permission: string(authz.ScanCreate), Tag: "diagnosis", Summary: "Retry a failed analysis",
			Response: Scan{}, Handler: h.retry},
		{Method: http.MethodPost, Path: "/v1/scans/sync", Permission: string(authz.ScanSync), Tag: "diagnosis", Summary: "Apply an offline operation queue",
			Request: SyncRequest{}, Response: SyncResponse{}, Handler: h.sync},
	}
}

// ToScan renders a scan.
func ToScan(s domain.Scan) Scan {
	out := Scan{ID: s.ID, MediaID: s.MediaID, FieldID: s.FieldID, CropCode: s.CropCode, Status: string(s.Status),
		CapturedAt: s.CapturedAt, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt, Note: s.Note, Saved: s.Saved}
	if d := s.Diagnosis; d != nil {
		out.Diagnosis = &Diagnosis{DiseaseCode: d.DiseaseCode, NameKey: "disease." + d.DiseaseCode + ".name",
			DescriptionKey: "disease." + d.DiseaseCode + ".description", Confidence: d.Confidence, Severity: d.Severity,
			Healthy: d.Healthy(), ModelVersion: d.ModelVersion, Plans: make([]Plan, len(d.Plans))}
		for i, p := range d.Plans {
			out.Diagnosis.Plans[i] = Plan{Variant: p.Variant, Steps: p.Steps, SafetyKey: p.SafetyKey}
		}
	}
	return out
}

func createInput(in CreateScanRequest) usecase.CreateInput {
	return usecase.CreateInput{MediaID: in.MediaID, CropCode: in.CropCode, FieldID: in.FieldID,
		IdempotencyKey: in.IdempotencyKey, CapturedAt: in.CapturedAt, Lang: in.Lang}
}

func (h Handlers) create(w http.ResponseWriter, r *http.Request) error {
	var in CreateScanRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	s, created, err := h.Create.Execute(r.Context(), createInput(in))
	if err != nil {
		return err
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return httpx.JSON(w, status, ToScan(s))
}

func (h Handlers) list(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	limit, err := httpx.QueryInt(r, "limit", usecase.DefaultListLimit, 1, usecase.MaxListLimit)
	if err != nil {
		return err
	}
	f := domain.ListFilter{OwnerID: q.Get("owner_id"), SavedOnly: q.Get("saved") == "true", Limit: int(limit)}
	if raw := q.Get("before"); raw != "" {
		if f.Before, err = time.Parse(time.RFC3339Nano, raw); err != nil {
			return httpx.ErrBadParam.WithField("before", "RFC 3339 time")
		}
	}
	ss, err := h.List.Execute(r.Context(), f)
	if err != nil {
		return err
	}
	out := ScanList{Scans: make([]Scan, len(ss))}
	for i, s := range ss {
		out.Scans[i] = ToScan(s)
	}
	if len(ss) == f.Limit {
		next := ss[len(ss)-1].CreatedAt
		out.NextBefore = &next
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h Handlers) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	s, err := h.Get.Execute(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, ToScan(s))
}

func (h Handlers) annotate(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in AnnotateRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	s, err := h.Annotate.Execute(r.Context(), id, usecase.AnnotateInput{Note: in.Note, Saved: in.Saved})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, ToScan(s))
}

func (h Handlers) retry(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	s, err := h.Retry.Execute(r.Context(), id, r.URL.Query().Get("lang"))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, ToScan(s))
}

func (h Handlers) sync(w http.ResponseWriter, r *http.Request) error {
	var in SyncRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	ops := make([]usecase.SyncOp, len(in.Operations))
	for i, o := range in.Operations {
		ops[i] = usecase.SyncOp{IdempotencyKey: o.IdempotencyKey, Seq: o.Seq, Kind: o.Kind, ScanID: o.ScanID}
		if o.Create != nil {
			c := createInput(*o.Create)
			ops[i].Create = &c
		}
		if o.Annotation != nil {
			ops[i].Annotation = usecase.AnnotateInput{Note: o.Annotation.Note, Saved: o.Annotation.Saved}
		}
	}
	res, err := h.Sync.Execute(r.Context(), ops)
	if err != nil {
		return err
	}
	out := SyncResponse{Results: make([]SyncResult, len(res))}
	for i, x := range res {
		out.Results[i] = SyncResult{IdempotencyKey: x.IdempotencyKey, Outcome: x.Outcome, ScanID: x.ScanID, ErrorCode: x.ErrorCode}
	}
	return httpx.JSON(w, http.StatusOK, out)
}
