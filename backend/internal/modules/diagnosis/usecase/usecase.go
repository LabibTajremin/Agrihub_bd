// Package usecase implements diagnosis: scan creation and analysis through the
// AI port, confidence routing, listing, annotation and idempotent offline sync.
package usecase

import (
	"context"
	"errors"
	"io"
	"sort"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/cache"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/tx"
)

// Event topics.
const (
	TopicScanCompleted = "diagnosis.scan.completed"
	TopicScanFailed    = "diagnosis.scan.failed"
)

// Deps are shared by the diagnosis use cases.
type Deps struct {
	Scans   domain.Scans
	Media   domain.Media
	Engine  aiadapter.DiagnosisEngine
	Cache   *cache.LRU[uint64, domain.Diagnosis] // keyed by image dHash (§6.8)
	Tx      tx.Manager
	Clock   clock.Clock
	IDs     idgen.Generator
	Events  eventbus.Publisher
	Factory eventbus.Factory
	// MinConfidence is the routing threshold (config ai.min_diagnosis_confidence).
	MinConfidence float64
	MaxImageBytes int64
}

// CreateInput requests a diagnosis of an uploaded photo.
type CreateInput struct {
	MediaID        string
	CropCode       string
	FieldID        string
	IdempotencyKey string
	CapturedAt     time.Time
	Lang           string
}

func (d Deps) publish(ctx context.Context, topic string, s domain.Scan) error {
	payload := map[string]any{"scan_id": s.ID, "owner_id": s.OwnerID, "field_id": s.FieldID, "crop_code": s.CropCode, "status": s.Status}
	if s.Diagnosis != nil {
		payload["disease_code"], payload["confidence"] = s.Diagnosis.DiseaseCode, s.Diagnosis.Confidence
	}
	e, err := d.Factory.New(ctx, topic, s.OwnerID, payload)
	if err == nil {
		err = d.Events.Publish(ctx, e)
	}
	return err
}

// CreateScan registers a scan (idempotent per owner+key) and analyses it.
type CreateScan struct{ Deps }

// Execute returns the scan and whether it was newly created.
func (uc CreateScan) Execute(ctx context.Context, in CreateInput) (domain.Scan, bool, error) {
	p, err := authn.Require(ctx, authz.ScanCreate)
	if err != nil {
		return domain.Scan{}, false, err
	}
	if in.IdempotencyKey == "" || in.CropCode == "" {
		return domain.Scan{}, false, domain.ErrInvalid.WithField("idempotency_key", "required")
	}
	existing, err := uc.Scans.GetByKey(ctx, p.Subject, in.IdempotencyKey)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.Scan{}, false, err
	}
	info, err := uc.Media.Info(ctx, in.MediaID)
	if err != nil || !info.Uploaded || info.OwnerID != p.Subject {
		return domain.Scan{}, false, domain.ErrInvalid.WithField("media_id", "unknown or not uploaded")
	}
	now := uc.Clock.Now()
	s := domain.Scan{ID: uc.IDs.New(), OwnerID: p.Subject, FieldID: in.FieldID, MediaID: in.MediaID, CropCode: in.CropCode,
		PHash: info.PHash, Status: domain.Queued, IdempotencyKey: in.IdempotencyKey, CapturedAt: in.CapturedAt, CreatedAt: now, UpdatedAt: now}
	if s.CapturedAt.IsZero() {
		s.CapturedAt = now
	}
	if err := uc.Scans.Create(ctx, s); err != nil {
		return domain.Scan{}, false, err
	}
	s, err = uc.analyse(ctx, s, info.ContentType, in.Lang)
	return s, true, err
}

// analyse runs the engine (or the phash cache) outside any transaction, then
// persists the routed result and its event atomically.
func (d Deps) analyse(ctx context.Context, s domain.Scan, contentType, lang string) (domain.Scan, error) {
	_ = s.Advance(domain.Analysing, d.Clock.Now()) // queued → analysing is always legal
	diag, hit := d.Cache.Get(s.PHash)
	topic := TopicScanCompleted
	if !hit {
		res, err := d.infer(ctx, s, contentType, lang)
		if err != nil {
			topic = TopicScanFailed
			_ = s.Advance(domain.Failed, d.Clock.Now())
		} else {
			diag = domain.Diagnosis{DiseaseCode: res.DiseaseCode, Confidence: res.Confidence, Severity: res.Severity,
				ModelVersion: res.ModelVersion, Plans: domain.Plans(res.DiseaseCode)}
			if s.PHash != 0 {
				d.Cache.Add(s.PHash, diag)
			}
		}
	}
	if topic == TopicScanCompleted {
		s.Diagnosis = &diag
		_ = s.Advance(domain.Route(diag.Confidence, d.MinConfidence), d.Clock.Now())
	}
	err := d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := d.Scans.SaveResult(ctx, s); err != nil {
			return err
		}
		return d.publish(ctx, topic, s)
	})
	return s, err
}

func (d Deps) infer(ctx context.Context, s domain.Scan, contentType, lang string) (aiadapter.AnalyzeResult, error) {
	rc, err := d.Media.Open(ctx, s.MediaID)
	if err != nil {
		return aiadapter.AnalyzeResult{}, err
	}
	defer rc.Close()
	img, err := io.ReadAll(io.LimitReader(rc, d.MaxImageBytes))
	if err != nil {
		return aiadapter.AnalyzeResult{}, err
	}
	return d.Engine.Analyze(ctx, aiadapter.AnalyzeInput{ScanID: s.ID, CropCode: s.CropCode, ContentType: contentType,
		Image: img, PHash: s.PHash, Lang: lang})
}

// Retry re-queues a failed scan and analyses it again.
type Retry struct{ Deps }

// Execute retries scan id (owner only).
func (uc Retry) Execute(ctx context.Context, id, lang string) (domain.Scan, error) {
	p, err := authn.Require(ctx, authz.ScanCreate)
	if err != nil {
		return domain.Scan{}, err
	}
	s, err := uc.Scans.Get(ctx, id)
	if err != nil {
		return domain.Scan{}, err
	}
	if s.OwnerID != p.Subject {
		return domain.Scan{}, authz.ErrForbidden
	}
	if err := s.Advance(domain.Queued, uc.Clock.Now()); err != nil {
		return domain.Scan{}, err
	}
	info, err := uc.Media.Info(ctx, s.MediaID)
	if err != nil {
		return domain.Scan{}, err
	}
	return uc.analyse(ctx, s, info.ContentType, lang)
}

// GetScan returns a scan (owner, or reviewers with scan:read_any).
type GetScan struct{ Deps }

// Execute loads scan id.
func (uc GetScan) Execute(ctx context.Context, id string) (domain.Scan, error) {
	p, err := authn.Require(ctx, authz.ScanRead)
	if err != nil {
		return domain.Scan{}, err
	}
	s, err := uc.Scans.Get(ctx, id)
	if err != nil {
		return domain.Scan{}, err
	}
	return s, authz.Owned(p.Role, p.Subject, s.OwnerID, authz.ScanRead, authz.ScanReadAny)
}

// List limits.
const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)

// ListScans lists scans newest first (history, or the saved log).
type ListScans struct{ Deps }

// Execute lists scans of f.OwnerID ("" = caller).
func (uc ListScans) Execute(ctx context.Context, f domain.ListFilter) ([]domain.Scan, error) {
	p, err := authn.Require(ctx, authz.ScanRead)
	if err != nil {
		return nil, err
	}
	if f.OwnerID == "" {
		f.OwnerID = p.Subject
	}
	if err := authz.Owned(p.Role, p.Subject, f.OwnerID, authz.ScanRead, authz.ScanReadAny); err != nil {
		return nil, err
	}
	if f.Limit <= 0 || f.Limit > MaxListLimit {
		f.Limit = DefaultListLimit
	}
	return uc.Scans.List(ctx, f)
}

// AnnotateInput holds optional annotation changes.
type AnnotateInput struct {
	Note  *string
	Saved *bool
}

// Annotate edits a scan's note / saved flag. Concurrent edits resolve
// last-write-wins by server-received time; the overwritten value is preserved
// in an audit row, never silently discarded.
type Annotate struct{ Deps }

// Execute applies the annotation to scan id (owner only).
func (uc Annotate) Execute(ctx context.Context, id string, in AnnotateInput) (domain.Scan, error) {
	p, err := authn.Require(ctx, authz.ScanCreate)
	if err != nil {
		return domain.Scan{}, err
	}
	var out domain.Scan
	err = uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		s, err := uc.Scans.Get(ctx, id)
		if err != nil {
			return err
		}
		if s.OwnerID != p.Subject {
			return authz.ErrForbidden
		}
		loser := domain.Annotation{Note: s.Note, Saved: s.Saved, UpdatedAt: s.UpdatedAt}
		winner := loser
		if in.Note != nil {
			winner.Note = *in.Note
		}
		if in.Saved != nil {
			winner.Saved = *in.Saved
		}
		winner.UpdatedAt = uc.Clock.Now()
		if winner.Note != loser.Note || winner.Saved != loser.Saved {
			if err := uc.Scans.RecordConflict(ctx, uc.IDs.New(), id, loser, winner, winner.UpdatedAt); err != nil {
				return err
			}
		}
		if err := uc.Scans.Annotate(ctx, id, winner); err != nil {
			return err
		}
		s.Note, s.Saved, s.UpdatedAt = winner.Note, winner.Saved, winner.UpdatedAt
		out = s
		return nil
	})
	return out, err
}

// Sync operation kinds.
const (
	OpCreateScan   = "create_scan"
	OpAnnotateScan = "annotate_scan"
)

// Sync outcomes.
const (
	OutcomeApplied   = "applied"
	OutcomeDuplicate = "duplicate"
	OutcomeRejected  = "rejected"
)

// SyncOp is one queued offline operation (§6.7).
type SyncOp struct {
	IdempotencyKey string
	Seq            int64
	Kind           string
	Create         *CreateInput
	ScanID         string
	Annotation     AnnotateInput
}

// SyncResult reports what happened to one operation.
type SyncResult struct {
	IdempotencyKey string
	Outcome        string
	ScanID         string
	ErrorCode      string
}

// Sync applies an offline queue in client sequence order. Each operation is
// deduplicated on its idempotency key, so replaying a batch is a no-op.
type Sync struct {
	Deps
	Create   CreateScan
	Annotate Annotate
}

// Execute processes ops and returns one result per op.
func (uc Sync) Execute(ctx context.Context, ops []SyncOp) ([]SyncResult, error) {
	p, err := authn.Require(ctx, authz.ScanSync)
	if err != nil {
		return nil, err
	}
	sorted := append([]SyncOp(nil), ops...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Seq < sorted[j].Seq })
	out := make([]SyncResult, 0, len(sorted))
	for _, op := range sorted {
		prev, err := uc.Scans.GetOp(ctx, p.Subject, op.IdempotencyKey)
		if err == nil {
			out = append(out, SyncResult{IdempotencyKey: op.IdempotencyKey, Outcome: OutcomeDuplicate, ScanID: prev.ScanID, ErrorCode: prev.ErrorCode})
			continue
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		scanID, err := uc.apply(ctx, op)
		res := SyncOutcome(op.IdempotencyKey, scanID, err)
		if res.Outcome == "" {
			return nil, err
		}
		if _, err := uc.Scans.ClaimOp(ctx, p.Subject, domain.SyncOutcome{IdempotencyKey: op.IdempotencyKey, ScanID: scanID,
			Outcome: res.Outcome, ErrorCode: res.ErrorCode}, uc.Clock.Now()); err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, nil
}

func (uc Sync) apply(ctx context.Context, op SyncOp) (string, error) {
	switch {
	case op.Kind == OpCreateScan && op.Create != nil:
		in := *op.Create
		in.IdempotencyKey = op.IdempotencyKey
		s, _, err := uc.Create.Execute(ctx, in)
		return s.ID, err
	case op.Kind == OpAnnotateScan:
		s, err := uc.Annotate.Execute(ctx, op.ScanID, op.Annotation)
		return s.ID, err
	default:
		return "", domain.ErrInvalid.WithField("kind", op.Kind)
	}
}

// SyncOutcome classifies an apply error: client errors reject the operation;
// server errors (internal/unavailable) yield an empty outcome so the whole
// batch fails and the client retries it later.
func SyncOutcome(key, scanID string, err error) SyncResult {
	if err == nil {
		return SyncResult{IdempotencyKey: key, Outcome: OutcomeApplied, ScanID: scanID}
	}
	e := errs.From(err)
	if e.Kind == errs.KindInternal || e.Kind == errs.KindUnavailable {
		return SyncResult{}
	}
	return SyncResult{IdempotencyKey: key, Outcome: OutcomeRejected, ScanID: scanID, ErrorCode: e.Code}
}
