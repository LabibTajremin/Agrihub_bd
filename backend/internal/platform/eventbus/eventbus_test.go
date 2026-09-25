package eventbus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/logger"
)

func factory() Factory {
	return Factory{IDs: &idgen.Sequence{}, Clock: clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))}
}

func TestFactory_StampsEnvelope(t *testing.T) {
	ctx := logger.WithRequestID(context.Background(), "rid")
	e, err := factory().New(ctx, "farm.field.created", "u1", map[string]int{"n": 1})
	if err != nil || e.TraceID != "rid" || e.ActorID != "u1" || e.Version != 1 || e.ID == "" || e.OccurredAt.IsZero() {
		t.Fatal(e, err)
	}
	var p map[string]int
	if err := e.Decode(&p); err != nil || p["n"] != 1 {
		t.Fatal(p, err)
	}
	if _, err := factory().New(ctx, "x.y.z", "", make(chan int)); err == nil {
		t.Fatal("unmarshalable payload must fail")
	}
}

func TestLocal_DispatchJoinsErrors(t *testing.T) {
	l := NewLocal()
	calls := 0
	l.Subscribe("a", func(context.Context, Event) error { calls++; return nil })
	l.Subscribe("a", func(context.Context, Event) error { calls++; return errors.New("h2") })
	if err := l.Dispatch(context.Background(), Event{Topic: "a"}); err == nil || calls != 2 {
		t.Fatal(err, calls)
	}
	if err := l.Dispatch(context.Background(), Event{Topic: "none"}); err != nil {
		t.Fatal(err)
	}
	if len(l.Topics()) != 1 {
		t.Fatal(l.Topics())
	}
}

func TestRecorder(t *testing.T) {
	r := &Recorder{}
	_ = r.Publish(context.Background(), Event{Topic: "a"})
	if r.Topics()[0] != "a" {
		t.Fatal()
	}
	r.Err = errors.New("x")
	if r.Publish(context.Background(), Event{}) == nil {
		t.Fatal()
	}
}
