//go:build integration

package outbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/outbox"
	"github.com/labibtajremin/agrihub_bd/backend/test/harness"
)

var (
	ctx = context.Background()
	t0  = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

func publish(t *testing.T, db *database.DB, f eventbus.Factory, topic string) eventbus.Event {
	t.Helper()
	e, err := f.New(ctx, topic, "actor", map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.NewPublisher(db).Publish(ctx, e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestRelay_DispatchesOnceAndRetriesFailures(t *testing.T) {
	db := harness.DB(t)
	c := clock.NewFake(t0)
	f := eventbus.Factory{IDs: &idgen.Sequence{}, Clock: c}
	bus := eventbus.NewLocal()
	var got []eventbus.Event
	bus.Subscribe("a.b.created", func(_ context.Context, e eventbus.Event) error { got = append(got, e); return nil })
	failures := 0
	bus.Subscribe("a.b.failed", func(context.Context, eventbus.Event) error { failures++; return errors.New("consumer down") })

	ok := publish(t, db, f, "a.b.created")
	publish(t, db, f, "a.b.failed")
	relay := outbox.NewRelay(db, bus, c, 10)

	res, err := relay.Flush(ctx)
	if err != nil || res.Dispatched != 1 || res.Failed != 1 {
		t.Fatal(res, err)
	}
	if len(got) != 1 || got[0].ID != ok.ID || got[0].ActorID != "actor" || !got[0].OccurredAt.Equal(t0) {
		t.Fatal(got)
	}
	res, _ = relay.Flush(ctx)
	if res.Dispatched != 0 || res.Failed != 1 || len(got) != 1 {
		t.Fatal("dispatched events are not redelivered; failed ones are retried", res)
	}
	for range outbox.MaxAttempts {
		_, _ = relay.Flush(ctx)
	}
	if failures != outbox.MaxAttempts {
		t.Fatalf("poison events stop after MaxAttempts, got %d", failures)
	}
	var lastErr string
	_ = db.Q(ctx).QueryRow(ctx, "SELECT last_error FROM outbox WHERE topic = 'a.b.failed'").Scan(&lastErr)
	if lastErr != "consumer down" {
		t.Fatal(lastErr)
	}
}

func TestRelay_StorageFaults(t *testing.T) {
	boom := errors.New("boom")
	c := clock.NewFake(t0)
	bus := eventbus.NewLocal()
	bus.Subscribe("x.y.failed", func(context.Context, eventbus.Event) error { return boom })
	cases := []harness.Fault{
		{Op: "query", Match: "FROM outbox", Err: boom},
		{Op: "exec", Match: "SET dispatched_at", Err: boom},
		{Op: "exec", Match: "last_error", Err: boom},
	}
	f := eventbus.Factory{IDs: idgen.UUIDv7{}, Clock: c}
	for _, fault := range cases {
		db := harness.FaultyDB(t, fault)
		publish(t, db, f, "x.y.created")
		publish(t, db, f, "x.y.failed")
		if _, err := outbox.NewRelay(db, bus, c, 10).Flush(ctx); !errors.Is(err, boom) {
			t.Errorf("%+v: want boom, got %v", fault, err)
		}
	}
}
