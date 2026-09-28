package clock

import (
	"testing"
	"time"
)

func TestSystem_ReturnsUTC(t *testing.T) {
	if loc := (System{}).Now().Location(); loc != time.UTC {
		t.Fatalf("want UTC, got %v", loc)
	}
}

func TestFake_AdvanceAndSet(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := NewFake(start)
	f.Advance(time.Minute)
	if !f.Now().Equal(start.Add(time.Minute)) {
		t.Fatal("advance failed")
	}
	f.Set(start)
	if !f.Now().Equal(start) {
		t.Fatal("set failed")
	}
}
