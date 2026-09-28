// Package stub is the deterministic Null-Object implementation of the three
// AI ports. It performs no inference: outputs are pure functions of the input
// so the whole app runs and tests at 100% with no model present.
package stub

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"strings"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter"
)

// ModelVersion identifies stub results.
const ModelVersion = "stub-1"

// Diseases the stub can report, in lookup order.
var Diseases = []string{"rice_blast", "brown_spot", "bacterial_leaf_blight", "sheath_blight", "tungro", "healthy"}

// Engine implements all three ports.
type Engine struct{}

var (
	_ aiadapter.DiagnosisEngine     = Engine{}
	_ aiadapter.AdvisoryNarrator    = Engine{}
	_ aiadapter.ConversationalAgent = Engine{}
)

// seed derives a stable 64-bit number from the image (dHash when present).
func seed(in aiadapter.AnalyzeInput) uint64 {
	if in.PHash != 0 {
		return in.PHash
	}
	sum := sha256.Sum256(in.Image)
	return binary.BigEndian.Uint64(sum[:8])
}

// Analyze picks a disease and a confidence in [0.50, 0.99] from the image seed.
func (Engine) Analyze(_ context.Context, in aiadapter.AnalyzeInput) (aiadapter.AnalyzeResult, error) {
	if len(in.Image) == 0 {
		return aiadapter.AnalyzeResult{}, aiadapter.ErrInvalidInput
	}
	s := seed(in)
	idx := int(s % uint64(len(Diseases))) //nolint:gosec // < len(Diseases)
	conf := 0.50 + float64((s>>8)%50)/100
	severity := []string{"low", "medium", "high"}[(s>>16)%3]
	alt := Diseases[(idx+1)%len(Diseases)]
	return aiadapter.AnalyzeResult{
		Finding:      aiadapter.Finding{DiseaseCode: Diseases[idx], Confidence: conf},
		Severity:     severity,
		Alternatives: []aiadapter.Finding{{DiseaseCode: alt, Confidence: (1 - conf) / 2}},
		ModelVersion: ModelVersion,
	}, nil
}

// ChartKinds the narrator knows.
var ChartKinds = []string{"scores", "forecast", "yield", "roi", "rotation", "weather"}

// Narrate returns the pre-written narration key for the chart kind; voice uses
// the same key (pre-recorded clip).
func (Engine) Narrate(_ context.Context, in aiadapter.NarrateInput) (aiadapter.NarrateResult, error) {
	if !slices.Contains(ChartKinds, in.ChartKind) {
		return aiadapter.NarrateResult{}, aiadapter.ErrInvalidInput
	}
	key := "narration.chart." + in.ChartKind
	return aiadapter.NarrateResult{TextKey: key, VoiceKey: key, Params: map[string]string{}}, nil
}

var intents = []struct {
	name     string
	keywords []string
}{
	{"weather", []string{"rain", "weather", "বৃষ্টি", "আবহাওয়া", "बारिश", "lluvia", "pluie", "مطر", "chuva"}},
	{"disease", []string{"disease", "sick", "wrong", "রোগ", "সমস্যা", "रोग", "समस्या", "enfermedad", "maladie", "مرض", "doença"}},
	{"crop", []string{"plant", "crop", "sow", "ফসল", "লাগাব", "फसल", "sembrar", "planter", "أزرع", "plantar"}},
}

// Ask matches the transcript against a small keyword table per language.
func (Engine) Ask(_ context.Context, in aiadapter.AskInput) (aiadapter.AskResult, error) {
	if in.Transcript == "" && in.AudioMediaID == "" {
		return aiadapter.AskResult{}, aiadapter.ErrInvalidInput
	}
	text := strings.ToLower(in.Transcript)
	for _, it := range intents {
		for _, kw := range it.keywords {
			if strings.Contains(text, kw) {
				return aiadapter.AskResult{Understood: true, Intent: it.name, AnswerKey: "voice.answer." + it.name,
					Params: map[string]string{}, FollowUps: followUps(it.name)}, nil
			}
		}
	}
	return aiadapter.AskResult{AnswerKey: "voice.not_understood", Params: map[string]string{}, FollowUps: followUps("")}, nil
}

func followUps(except string) []string {
	var out []string
	for _, it := range intents {
		if it.name != except {
			out = append(out, "voice.suggestion."+it.name)
		}
	}
	return out
}
