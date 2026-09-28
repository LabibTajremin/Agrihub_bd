package stub

import (
	"context"
	"errors"
	"testing"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter"
)

var ctx = context.Background()

func TestAnalyze_DeterministicAndBounded(t *testing.T) {
	e := Engine{}
	seen := map[string]bool{}
	low, high := false, false
	for i := range 200 {
		in := aiadapter.AnalyzeInput{Image: []byte{byte(i), byte(i >> 8), 7}, PHash: uint64(i) * 0x9E3779B97F4A7C15}
		a, err := e.Analyze(ctx, in)
		b, _ := e.Analyze(ctx, in)
		if err != nil || a.DiseaseCode != b.DiseaseCode || a.Confidence != b.Confidence {
			t.Fatal("must be deterministic")
		}
		if a.Confidence < 0.5 || a.Confidence > 0.99 || a.ModelVersion != ModelVersion || len(a.Alternatives) != 1 {
			t.Fatalf("%+v", a)
		}
		seen[a.DiseaseCode] = true
		low = low || a.Confidence < 0.6
		high = high || a.Confidence >= 0.6
	}
	if len(seen) != len(Diseases) || !low || !high {
		t.Fatal("stub must exercise every label and both confidence routes", seen)
	}
	noHash, err := e.Analyze(ctx, aiadapter.AnalyzeInput{Image: []byte("leaf")})
	if err != nil || noHash.DiseaseCode == "" {
		t.Fatal(err)
	}
	if _, err := e.Analyze(ctx, aiadapter.AnalyzeInput{}); !errors.Is(err, aiadapter.ErrInvalidInput) {
		t.Fatal(err)
	}
}

func TestNarrate(t *testing.T) {
	for _, k := range ChartKinds {
		r, err := Engine{}.Narrate(ctx, aiadapter.NarrateInput{ChartKind: k, Lang: "bn"})
		if err != nil || r.TextKey != "narration.chart."+k || r.VoiceKey != r.TextKey {
			t.Fatal(r, err)
		}
	}
	if _, err := (Engine{}).Narrate(ctx, aiadapter.NarrateInput{ChartKind: "pie"}); !errors.Is(err, aiadapter.ErrInvalidInput) {
		t.Fatal(err)
	}
}

func TestAsk(t *testing.T) {
	cases := map[string]string{
		"Will it rain this week?": "weather",
		"আমার ধানের কী রোগ?":      "disease",
		"What should I plant?":    "crop",
		"مطر":                     "weather",
	}
	for q, intent := range cases {
		r, err := Engine{}.Ask(ctx, aiadapter.AskInput{Transcript: q})
		if err != nil || !r.Understood || r.Intent != intent || r.AnswerKey != "voice.answer."+intent || len(r.FollowUps) != 2 {
			t.Errorf("%q: %+v %v", q, r, err)
		}
	}
	r, err := Engine{}.Ask(ctx, aiadapter.AskInput{AudioMediaID: "m"})
	if err != nil || r.Understood || r.AnswerKey != "voice.not_understood" || len(r.FollowUps) != 3 {
		t.Fatal(r, err)
	}
	if _, err := (Engine{}).Ask(ctx, aiadapter.AskInput{}); !errors.Is(err, aiadapter.ErrInvalidInput) {
		t.Fatal(err)
	}
	if len(aiadapter.Errors()) != 2 {
		t.Fatal()
	}
}
