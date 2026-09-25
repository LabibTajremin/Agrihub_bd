// Package outbox implements the transactional outbox: Publisher writes events
// in the caller's transaction; Relay later dispatches pending events to the
// in-process bus, one savepoint per event so a failing consumer is isolated.
package outbox

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
)

// MaxAttempts is the retry ceiling; poisoned events stay in the table for
// inspection instead of blocking the relay.
const MaxAttempts = 10

// Publisher implements eventbus.Publisher on the outbox table.
type Publisher struct{ db *database.DB }

// NewPublisher returns a Publisher bound to db.
func NewPublisher(db *database.DB) *Publisher { return &Publisher{db: db} }

// Publish inserts e using the transaction in ctx (if any).
func (p *Publisher) Publish(ctx context.Context, e eventbus.Event) error {
	_, err := p.db.Q(ctx).Exec(ctx, `INSERT INTO outbox (id, topic, payload, actor_id, trace_id, version, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		e.ID, e.Topic, []byte(e.Payload), e.ActorID, e.TraceID, e.Version, e.OccurredAt)
	return err
}

// Dispatcher delivers one event to its consumers.
type Dispatcher interface {
	Dispatch(ctx context.Context, e eventbus.Event) error
}

// Relay moves pending outbox rows to the dispatcher.
type Relay struct {
	db        *database.DB
	bus       Dispatcher
	clock     clock.Clock
	batchSize int
}

// NewRelay returns a Relay.
func NewRelay(db *database.DB, bus Dispatcher, c clock.Clock, batchSize int) *Relay {
	return &Relay{db: db, bus: bus, clock: c, batchSize: batchSize}
}

// Result summarises one Flush.
type Result struct {
	Dispatched int
	Failed     int
}

// Flush dispatches up to batchSize pending events. Rows are locked with
// SKIP LOCKED so concurrent relays (several instances) never double-deliver
// within a flush.
func (r *Relay) Flush(ctx context.Context) (Result, error) {
	var res Result
	err := r.db.WithinTx(ctx, func(ctx context.Context) error {
		events, err := r.pending(ctx)
		if err != nil {
			return err
		}
		for _, e := range events {
			derr := r.db.WithinTx(ctx, func(ctx context.Context) error { return r.bus.Dispatch(ctx, e) })
			if derr != nil {
				res.Failed++
				if _, err := r.db.Q(ctx).Exec(ctx,
					`UPDATE outbox SET attempts = attempts + 1, last_error = $2 WHERE id = $1`, e.ID, derr.Error()); err != nil {
					return err
				}
				continue
			}
			res.Dispatched++
			if _, err := r.db.Q(ctx).Exec(ctx,
				`UPDATE outbox SET dispatched_at = $2, attempts = attempts + 1 WHERE id = $1`, e.ID, r.clock.Now()); err != nil {
				return err
			}
		}
		return nil
	})
	return res, err
}

func (r *Relay) pending(ctx context.Context) ([]eventbus.Event, error) {
	return database.Collect(ctx, r.db.Q(ctx), scanEvent, `SELECT id, topic, payload, actor_id, trace_id, version, occurred_at
		FROM outbox WHERE dispatched_at IS NULL AND attempts < $1
		ORDER BY occurred_at, id LIMIT $2 FOR UPDATE SKIP LOCKED`, MaxAttempts, r.batchSize)
}

func scanEvent(row pgx.CollectableRow) (eventbus.Event, error) {
	var e eventbus.Event
	var payload []byte
	err := row.Scan(&e.ID, &e.Topic, &payload, &e.ActorID, &e.TraceID, &e.Version, &e.OccurredAt)
	e.Payload = json.RawMessage(payload)
	e.OccurredAt = e.OccurredAt.UTC()
	return e, err
}
