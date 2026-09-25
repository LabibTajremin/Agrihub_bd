// Package aiadapter is the OPEN SLOT for the farming model. It declares the
// three ports the rest of the system depends on and nothing else; the only
// implementation shipped is the deterministic stub in ./stub. See
// docs/AI_INTEGRATION.md for the contract and how to plug a real model in.
package aiadapter

import (
	"context"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// AnalyzeInput is one leaf photo to diagnose.
type AnalyzeInput struct {
	ScanID      string
	CropCode    string // farm catalogue code, e.g. "rice_aman"
	ContentType string // image/jpeg or image/png
	Image       []byte // the verified upload
	PHash       uint64 // 64-bit dHash of Image
	Lang        string // caller's language (for any text the model returns)
}

// Finding is one candidate label.
type Finding struct {
	DiseaseCode string  // e.g. "rice_blast", "healthy"
	Confidence  float64 // [0, 1]
}

// AnalyzeResult is the model's verdict. Confidence routing (≥ threshold →
// treatment plan) is applied by the diagnosis module, not the model.
type AnalyzeResult struct {
	Finding
	Severity     string    // "low" | "medium" | "high"
	Alternatives []Finding // next-best labels, highest first
	ModelVersion string
}

// DiagnosisEngine classifies a leaf photo.
type DiagnosisEngine interface {
	Analyze(ctx context.Context, in AnalyzeInput) (AnalyzeResult, error)
}

// NarrateInput describes a chart to narrate.
type NarrateInput struct {
	ChartKind string             // "scores" | "forecast" | "yield" | "roi" | "rotation" | "weather"
	Lang      string             // language code
	Data      map[string]float64 // the plotted values, by series label
}

// NarrateResult is a narration: a dictionary key (so text and pre-recorded
// voice come from the localization module) plus optional generated text.
type NarrateResult struct {
	TextKey  string
	VoiceKey string
	Params   map[string]string
	Text     string // free text from a generative model; empty for the stub
}

// AdvisoryNarrator explains charts in the user's language.
type AdvisoryNarrator interface {
	Narrate(ctx context.Context, in NarrateInput) (NarrateResult, error)
}

// AskInput is one voice-assistant turn. There is no STT in the system:
// Transcript comes from the client (typed or chosen suggestion), or is empty
// when only AudioMediaID is provided for a future speech model.
type AskInput struct {
	Lang         string
	Transcript   string
	AudioMediaID string
	Context      map[string]string // e.g. "screen": "home"
}

// AskResult is the assistant's answer as a dictionary key.
type AskResult struct {
	Understood bool
	Intent     string // "weather" | "disease" | "crop" | ""
	AnswerKey  string // dictionary key of the answer
	Params     map[string]string
	FollowUps  []string // dictionary keys of suggested next questions
}

// ConversationalAgent answers voice questions.
type ConversationalAgent interface {
	Ask(ctx context.Context, in AskInput) (AskResult, error)
}

// Errors every implementation must use.
var (
	ErrUnavailable  = errs.Unavailable("ai.unavailable")
	ErrInvalidInput = errs.Validation("ai.invalid_input")
)

// Errors lists the AI error catalogue.
func Errors() []*errs.Error { return []*errs.Error{ErrUnavailable, ErrInvalidInput} }
