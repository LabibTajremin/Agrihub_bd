// Package eventbus defines domain events and an in-process dispatcher. Writes
// across module boundaries travel as events (via the outbox), never as calls.
package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/logger"
)

// Event is the envelope every event carries. Topic naming:
// <module>.<entity>.<past_tense_verb>, e.g. diagnosis.scan.completed.
type Event struct {
	ID         string          `json:"id"`
	Topic      string          `json:"topic"`
	OccurredAt time.Time       `json:"occurred_at"`
	ActorID    string          `json:"actor_id"`
	TraceID    string          `json:"trace_id"`
	Version    int             `json:"version"`
	Payload    json.RawMessage `json:"payload"`
}

// Decode unmarshals the payload into v.
func (e Event) Decode(v any) error { return json.Unmarshal(e.Payload, v) }

// Publisher records an event for delivery (transactionally via the outbox).
type Publisher interface {
	Publish(ctx context.Context, e Event) error
}

// Handler consumes an event. Handlers must be idempotent (at-least-once).
type Handler func(ctx context.Context, e Event) error

// Factory stamps envelopes with IDs, time and the request trace ID.
type Factory struct {
	IDs   idgen.Generator
	Clock clock.Clock
}

// New builds an event with a JSON payload.
func (f Factory) New(ctx context.Context, topic, actorID string, payload any) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	return Event{
		ID: f.IDs.New(), Topic: topic, OccurredAt: f.Clock.Now(), ActorID: actorID,
		TraceID: logger.RequestID(ctx), Version: 1, Payload: raw,
	}, nil
}

// Local routes events to in-process subscribers.
type Local struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

// NewLocal returns an empty dispatcher.
func NewLocal() *Local { return &Local{handlers: map[string][]Handler{}} }

// Subscribe registers h for topic.
func (l *Local) Subscribe(topic string, h Handler) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.handlers[topic] = append(l.handlers[topic], h)
}

// Topics lists topics with at least one subscriber.
func (l *Local) Topics() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]string, 0, len(l.handlers))
	for t := range l.handlers {
		out = append(out, t)
	}
	return out
}

// Dispatch runs every handler for e.Topic and joins their errors.
func (l *Local) Dispatch(ctx context.Context, e Event) error {
	l.mu.RLock()
	hs := l.handlers[e.Topic]
	l.mu.RUnlock()
	var all []error
	for _, h := range hs {
		if err := h(ctx, e); err != nil {
			all = append(all, err)
		}
	}
	return errors.Join(all...)
}

// Recorder is an in-memory Publisher for unit tests of publishing use cases.
type Recorder struct {
	mu     sync.Mutex
	Events []Event
	Err    error
}

// Publish implements Publisher.
func (r *Recorder) Publish(_ context.Context, e Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.Events = append(r.Events, e)
	return nil
}

// Topics returns the recorded topics in order.
func (r *Recorder) Topics() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.Events))
	for i, e := range r.Events {
		out[i] = e.Topic
	}
	return out
}
