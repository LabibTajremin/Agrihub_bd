// Package domain holds the diagnosis model: the scan lifecycle state machine,
// confidence routing and the treatment catalogue.
package domain

import (
	"context"
	"io"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// Status is a scan lifecycle state.
type Status string

// Statuses.
const (
	Queued        Status = "queued"
	Analysing     Status = "analysing"
	Completed     Status = "completed"
	LowConfidence Status = "low_confidence"
	Failed        Status = "failed"
)

// Statuses lists every status.
func Statuses() []Status { return []Status{Queued, Analysing, Completed, LowConfidence, Failed} }

// transitions is the complete legal-transition table; anything absent is illegal.
var transitions = map[Status][]Status{
	Queued:    {Analysing},
	Analysing: {Completed, LowConfidence, Failed},
	Failed:    {Queued}, // retry
}

// Errors.
var (
	ErrNotFound          = errs.NotFound("scan.not_found")
	ErrInvalidTransition = errs.Conflict("scan.invalid_transition")
	ErrInvalid           = errs.Validation("scan.invalid")
	ErrAnalysisFailed    = errs.Unavailable("scan.analysis_failed")
)

// Errors lists the diagnosis error catalogue.
func Errors() []*errs.Error {
	return []*errs.Error{ErrNotFound, ErrInvalidTransition, ErrInvalid, ErrAnalysisFailed}
}

// Transition validates from → to.
func Transition(from, to Status) error {
	for _, s := range transitions[from] {
		if s == to {
			return nil
		}
	}
	return ErrInvalidTransition.WithField("transition", string(from)+"->"+string(to))
}

// Route applies the confidence threshold (§6.9): at or above it the treatment
// plan is shown; below it the low-confidence screen and escalation path.
func Route(confidence, threshold float64) Status {
	if confidence >= threshold {
		return Completed
	}
	return LowConfidence
}

// TreatmentPlan is one treatment variant; steps are dictionary keys.
type TreatmentPlan struct {
	Variant   string   `json:"variant"` // "chemical" | "organic"
	Steps     []string `json:"steps"`
	SafetyKey string   `json:"safety_key,omitempty"`
}

// Diagnosis is the analysed result of a scan.
type Diagnosis struct {
	DiseaseCode  string          `json:"disease_code"`
	Confidence   float64         `json:"confidence"`
	Severity     string          `json:"severity"`
	ModelVersion string          `json:"model_version"`
	Plans        []TreatmentPlan `json:"plans"`
}

// Healthy reports whether no disease was found.
func (d Diagnosis) Healthy() bool { return d.DiseaseCode == "healthy" }

// Plans returns the treatment plans for a disease (chemical and organic).
func Plans(disease string) []TreatmentPlan {
	if disease == "healthy" {
		return []TreatmentPlan{{Variant: "organic", Steps: []string{"treatment.healthy.step1"}}}
	}
	base := "treatment." + disease + "."
	return []TreatmentPlan{
		{Variant: "chemical", Steps: []string{base + "chemical.step1", base + "chemical.step2"}, SafetyKey: "treatment.safety"},
		{Variant: "organic", Steps: []string{base + "organic.step1", base + "organic.step2"}},
	}
}

// Scan is one diagnosis request.
type Scan struct {
	ID             string
	OwnerID        string
	FieldID        string
	MediaID        string
	CropCode       string
	PHash          uint64
	Status         Status
	IdempotencyKey string
	CapturedAt     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Note           string
	Saved          bool
	Diagnosis      *Diagnosis
}

// Advance moves the scan to status, rejecting illegal transitions.
func (s *Scan) Advance(to Status, at time.Time) error {
	if err := Transition(s.Status, to); err != nil {
		return err
	}
	s.Status, s.UpdatedAt = to, at
	return nil
}

// Annotation is the user-editable part of a scan (last-write-wins on sync).
type Annotation struct {
	Note      string    `json:"note"`
	Saved     bool      `json:"saved"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListFilter narrows a scan listing.
type ListFilter struct {
	OwnerID   string
	SavedOnly bool
	Before    time.Time // exclusive cursor on created_at; zero = newest
	Limit     int
}

// SyncOutcome is the recorded result of one offline operation.
type SyncOutcome struct {
	IdempotencyKey string
	ScanID         string
	Outcome        string // "applied" | "rejected"
	ErrorCode      string
}

// Scans is the scan repository port.
type Scans interface {
	Create(ctx context.Context, s Scan) error
	Get(ctx context.Context, id string) (Scan, error)
	GetByKey(ctx context.Context, ownerID, key string) (Scan, error)
	List(ctx context.Context, f ListFilter) ([]Scan, error)
	SaveResult(ctx context.Context, s Scan) error
	Annotate(ctx context.Context, id string, a Annotation) error
	RecordConflict(ctx context.Context, id, scanID string, loser, winner Annotation, at time.Time) error
	// ClaimOp stores an operation's outcome; false means the key was already processed.
	ClaimOp(ctx context.Context, ownerID string, o SyncOutcome, at time.Time) (bool, error)
	GetOp(ctx context.Context, ownerID, key string) (SyncOutcome, error)
}

// MediaInfo is what diagnosis needs to know about an uploaded photo.
type MediaInfo struct {
	OwnerID     string
	ContentType string
	PHash       uint64
	Uploaded    bool
}

// Media is the consumer-side port onto the media module.
type Media interface {
	Info(ctx context.Context, id string) (MediaInfo, error)
	Open(ctx context.Context, id string) (io.ReadCloser, error)
}
