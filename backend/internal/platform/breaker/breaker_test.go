package breaker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
)

var errDown = errors.New("down")

// TestBreaker_Transitions drives the breaker with a table of injected faults
// and asserts state after every step: closed → open → half-open → closed/open.
func TestBreaker_Transitions(t *testing.T) {
	type step struct {
		advance time.Duration
		fail    bool
		wantErr error
		state   State
		calls   int // cumulative calls that reached the dependency
	}
	steps := []step{
		{fail: true, wantErr: errDown, state: Closed, calls: 1},
		{fail: false, state: Closed, calls: 2},                  // success resets the count
		{fail: true, wantErr: errDown, state: Closed, calls: 3}, // 1st consecutive
		{fail: true, wantErr: errDown, state: Closed, calls: 4}, // 2nd
		{fail: true, wantErr: errDown, state: Open, calls: 5},   // 3rd → opens
		{fail: false, wantErr: ErrOpen, state: Open, calls: 5},  // fails fast
		{advance: 29 * time.Second, wantErr: ErrOpen, state: Open, calls: 5},
		{advance: time.Second, fail: true, wantErr: errDown, state: Open, calls: 6}, // half-open probe fails → open again
		{advance: 30 * time.Second, fail: false, state: Closed, calls: 7},           // probe succeeds → closed
	}
	c := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	b := New(Options{Threshold: 3, OpenTimeout: 30 * time.Second, HalfOpenMax: 1, Clock: c})
	calls := 0
	for i, s := range steps {
		c.Advance(s.advance)
		err := b.Do(context.Background(), func(context.Context) error {
			calls++
			if s.fail {
				return errDown
			}
			return nil
		})
		if !errors.Is(err, s.wantErr) && (s.wantErr != nil || err != nil) {
			t.Fatalf("step %d: err %v want %v", i, err, s.wantErr)
		}
		if b.State() != s.state || calls != s.calls {
			t.Fatalf("step %d: state %s calls %d, want %s %d", i, b.State(), calls, s.state, s.calls)
		}
	}
}

func TestBreaker_HalfOpenLimitsProbes(t *testing.T) {
	c := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	b := New(Options{Threshold: 1, OpenTimeout: time.Second, HalfOpenMax: 1, Clock: c})
	_ = b.Do(context.Background(), func(context.Context) error { return errDown })
	c.Advance(time.Second)
	if b.State() != HalfOpen || b.State().String() != "half_open" {
		t.Fatal(b.State())
	}
	inner := b.Do(context.Background(), func(ctx context.Context) error {
		// a concurrent call while the only probe is in flight fails fast
		return b.Do(ctx, func(context.Context) error { return nil })
	})
	if !errors.Is(inner, ErrOpen) {
		t.Fatal(inner)
	}
	if Closed.String() != "closed" || Open.String() != "open" {
		t.Fatal()
	}
}
