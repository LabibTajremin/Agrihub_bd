// Package breaker is a circuit breaker for outbound calls: after Threshold
// consecutive failures it opens and fails fast; after OpenTimeout it lets up
// to HalfOpenMax probe calls through; a successful probe closes it again.
package breaker

import (
	"context"
	"sync"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// State is the breaker state.
type State int

// States.
const (
	Closed State = iota
	Open
	HalfOpen
)

func (s State) String() string { return [...]string{"closed", "open", "half_open"}[s] }

// ErrOpen is returned without calling fn while the breaker is open.
var ErrOpen = errs.Unavailable("breaker.open")

// Options configures a breaker.
type Options struct {
	Threshold   int
	OpenTimeout time.Duration
	HalfOpenMax int
	Clock       clock.Clock
}

// Breaker guards calls to one dependency.
type Breaker struct {
	mu       sync.Mutex
	o        Options
	state    State
	failures int
	openedAt time.Time
	inFlight int
}

// New returns a closed breaker.
func New(o Options) *Breaker { return &Breaker{o: o} }

// State reports the current state (an expired open state reads as half-open).
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	return b.state
}

func (b *Breaker) expire() {
	if b.state == Open && !b.o.Clock.Now().Before(b.openedAt.Add(b.o.OpenTimeout)) {
		b.state, b.inFlight = HalfOpen, 0
	}
}

func (b *Breaker) allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	switch b.state {
	case Closed: // calls flow freely
	case Open:
		return ErrOpen
	case HalfOpen:
		if b.inFlight >= b.o.HalfOpenMax {
			return ErrOpen
		}
		b.inFlight++
	}
	return nil
}

func (b *Breaker) record(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		b.state, b.failures, b.inFlight = Closed, 0, 0
		return
	}
	b.failures++
	if b.state == HalfOpen || b.failures >= b.o.Threshold {
		b.state, b.openedAt, b.inFlight = Open, b.o.Clock.Now(), 0
	}
}

// Do runs fn through the breaker.
func (b *Breaker) Do(ctx context.Context, fn func(context.Context) error) error {
	if err := b.allow(); err != nil {
		return err
	}
	err := fn(ctx)
	b.record(err)
	return err
}
