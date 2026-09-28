package domain

import (
	"errors"
	"testing"
	"time"
)

// TestTransition_ExhaustiveTable checks every (from, to) pair against the
// legal-transition table.
func TestTransition_ExhaustiveTable(t *testing.T) {
	legal := map[[2]Status]bool{
		{Queued, Analysing}: true, {Analysing, Completed}: true, {Analysing, LowConfidence}: true,
		{Analysing, Failed}: true, {Failed, Queued}: true,
	}
	for _, from := range Statuses() {
		for _, to := range Statuses() {
			err := Transition(from, to)
			if legal[[2]Status{from, to}] != (err == nil) {
				t.Errorf("%s -> %s: legal=%v err=%v", from, to, legal[[2]Status{from, to}], err)
			}
			if err != nil && !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("typed error expected")
			}
		}
	}
	if Transition("bogus", Queued) == nil {
		t.Fatal("unknown state")
	}
}

// TestRoute_Boundaries covers §6.9: 0.599 / 0.600 / 0.601.
func TestRoute_Boundaries(t *testing.T) {
	cases := map[float64]Status{0.599: LowConfidence, 0.600: Completed, 0.601: Completed, 0: LowConfidence, 1: Completed}
	for c, want := range cases {
		if got := Route(c, 0.60); got != want {
			t.Errorf("%v: %s", c, got)
		}
	}
}

func TestPlansAndAdvance(t *testing.T) {
	p := Plans("rice_blast")
	if len(p) != 2 || p[0].Variant != "chemical" || p[0].Steps[0] != "treatment.rice_blast.chemical.step1" || p[1].SafetyKey != "" {
		t.Fatal(p)
	}
	if h := Plans("healthy"); len(h) != 1 || !(Diagnosis{DiseaseCode: "healthy"}).Healthy() {
		t.Fatal(h)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := Scan{Status: Queued}
	if err := s.Advance(Analysing, now); err != nil || s.Status != Analysing || !s.UpdatedAt.Equal(now) {
		t.Fatal(err)
	}
	if err := s.Advance(Queued, now); err == nil {
		t.Fatal("illegal")
	}
	if len(Errors()) != 4 {
		t.Fatal()
	}
}
