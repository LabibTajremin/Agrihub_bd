//go:build integration

package alert_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/logger"
	"github.com/labibtajremin/agrihub_bd/backend/test/apitest"
	"github.com/labibtajremin/agrihub_bd/backend/test/harness"
)

var (
	ctx  = context.Background()
	boom = errors.New("boom")
)

const (
	alice = "00000000-0000-7000-8000-0000000000a1"
	bob   = "00000000-0000-7000-8000-0000000000b2"
)

type notifier struct{ err error }

func (n notifier) Notify(context.Context, domain.Alert) error { return n.err }

func setup(t *testing.T, db *database.DB, n domain.Notifier) *apitest.API {
	t.Helper()
	a := apitest.New(t, db)
	m, err := alert.New(alert.Deps{Kernel: a.Kernel, Notifier: n})
	if err != nil {
		t.Fatal(err)
	}
	a.Mount(m.Routes())
	return a
}

func publish(t *testing.T, a *apitest.API, topic string, payload any) eventbus.Event {
	t.Helper()
	e, err := a.Kernel.Factory().New(ctx, topic, "", payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Kernel.Events.Publish(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Relay.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	return e
}

func forecast(rainMM10, tmaxC10 int) map[string]any {
	return map[string]any{"cell_key": "24.9:89.4", "days": []domain.Day{{RainMM10: rainMM10, TempMaxC10: tmaxC10, TempMinC10: 220, HumidityPct: 60}}}
}

func inbox(t *testing.T, a *apitest.API, tok, query string) transport.Inbox {
	t.Helper()
	var in transport.Inbox
	a.Do("GET", "/v1/alerts"+query, nil, tok).Expect(t, 200).Decode(t, &in)
	return in
}

func TestWeatherEvents_RaiseAlertsForSubscribersOnce(t *testing.T) {
	a := setup(t, harness.DB(t), nil)
	at, bt := a.Token(alice, authz.Farmer), a.Token(bob, authz.Farmer)
	a.Do("GET", "/v1/alerts/subscription", nil, at).Expect(t, 404)
	var sub transport.Subscription
	a.Do("PUT", "/v1/alerts/subscription", transport.SubscriptionRequest{Lat: 24.85, Lng: 89.37, Kinds: []string{"heavy_rain", "heat"}}, at).
		Expect(t, 200).Decode(t, &sub)
	if sub.CellKey != "24.9:89.4" {
		t.Fatal(sub)
	}
	var stored transport.Subscription
	a.Do("GET", "/v1/alerts/subscription", nil, at).Expect(t, 200).Decode(t, &stored)
	if len(stored.Kinds) != 2 || stored.Lat != 24.85 {
		t.Fatal(stored)
	}
	a.Do("PUT", "/v1/alerts/subscription", transport.SubscriptionRequest{Lat: 24.86, Lng: 89.41, Kinds: []string{"heat"}}, bt).Expect(t, 200)

	e := publish(t, a, usecase.TopicForecastUpdated, forecast(720, 300))
	got := inbox(t, a, at, "")
	if len(got.Alerts) != 1 || got.UnreadCount != 1 || got.Alerts[0].Kind != "heavy_rain" || got.Alerts[0].Params["mm"] != "72" ||
		got.Alerts[0].TitleKey != "alerts.heavy_rain.title" {
		t.Fatalf("%+v", got)
	}
	if len(inbox(t, a, bt, "").Alerts) != 0 {
		t.Fatal("bob subscribed to heat only")
	}
	// redelivery of the same event (at-least-once) is idempotent
	if err := a.Kernel.Bus.Dispatch(ctx, e); err != nil {
		t.Fatal(err)
	}
	if len(inbox(t, a, at, "").Alerts) != 1 {
		t.Fatal("duplicate alert on redelivery")
	}
	publish(t, a, usecase.TopicForecastUpdated, forecast(0, 390))
	if in := inbox(t, a, bt, ""); len(in.Alerts) != 1 || in.Alerts[0].Severity != "critical" {
		t.Fatalf("%+v", in)
	}
	publish(t, a, usecase.TopicForecastUpdated, forecast(0, 300)) // calm: nothing
	if n := len(inbox(t, a, at, "").Alerts); n != 2 {
		t.Fatal(n)
	}

	id := got.Alerts[0].ID
	var one transport.Alert
	a.Do("GET", "/v1/alerts/"+id, nil, at).Expect(t, 200).Decode(t, &one)
	a.Do("GET", "/v1/alerts/"+id, nil, bt).Expect(t, 404)
	a.Do("POST", "/v1/alerts/"+id+"/read", nil, at).Expect(t, 200).Decode(t, &one)
	if one.ReadAt == nil {
		t.Fatal("read")
	}
	a.Do("POST", "/v1/alerts/"+id+"/read", nil, at).Expect(t, 200)
	if in := inbox(t, a, at, "?unread=true"); len(in.Alerts) != 1 || in.UnreadCount != 1 {
		t.Fatalf("%+v", in)
	}
	var ra transport.ReadAll
	a.Do("POST", "/v1/alerts/read-all", nil, at).Expect(t, 200).Decode(t, &ra)
	if ra.Updated != 1 || inbox(t, a, at, "").UnreadCount != 0 {
		t.Fatal(ra)
	}
	a.Do("DELETE", "/v1/alerts/subscription", nil, at).Expect(t, 204)
	a.Do("GET", "/v1/alerts/subscription", nil, at).Expect(t, 404)
}

func TestScanEvents_FollowupForDiseaseOnly(t *testing.T) {
	a := setup(t, harness.DB(t), notifier{err: boom}) // notifier failures are logged, not fatal
	tok := a.Token(alice, authz.Farmer)
	publish(t, a, usecase.TopicScanCompleted, map[string]any{"owner_id": alice, "status": "completed", "disease_code": "healthy"})
	publish(t, a, usecase.TopicScanCompleted, map[string]any{"owner_id": alice, "status": "low_confidence", "disease_code": "tungro"})
	publish(t, a, usecase.TopicScanCompleted, map[string]any{"owner_id": alice, "status": "completed", "disease_code": "tungro"})
	in := inbox(t, a, tok, "")
	if len(in.Alerts) != 1 || in.Alerts[0].Kind != "disease_followup" || in.Alerts[0].Params["disease"] != "disease.tungro.name" {
		t.Fatalf("%+v", in)
	}
}

func TestHTTP_ValidationAndAccess(t *testing.T) {
	a := setup(t, harness.DB(t), nil)
	tok := a.Token(alice, authz.Farmer)
	a.Do("PUT", "/v1/alerts/subscription", transport.SubscriptionRequest{Lat: 1, Lng: 1, Kinds: []string{"flood"}}, tok).Expect(t, 400)
	a.Do("PUT", "/v1/alerts/subscription", transport.SubscriptionRequest{Lat: 1, Lng: 1}, tok).Expect(t, 400)
	a.Do("PUT", "/v1/alerts/subscription", "{", tok).Expect(t, 400)
	a.Do("GET", "/v1/alerts", nil, a.Token(alice, authz.Guest)).Expect(t, 403)
	a.Do("GET", "/v1/alerts?limit=0", nil, tok).Expect(t, 400)
	a.Do("GET", "/v1/alerts/nope", nil, tok).Expect(t, 400)
	a.Do("POST", "/v1/alerts/nope/read", nil, tok).Expect(t, 400)
	a.Do("POST", "/v1/alerts/00000000-0000-7000-8000-000000000999/read", nil, tok).Expect(t, 404)
}

func deps(t *testing.T, db *database.DB) usecase.Deps {
	a := apitest.New(t, db)
	return usecase.Deps{Alerts: repository.Alerts{DB: db}, Subscriptions: repository.Subscriptions{DB: db}, Notifier: notifier{},
		Clock: a.Clock, IDs: a.Kernel.IDs, Logger: logger.Discard()}
}

var farmer = authn.WithPrincipal(ctx, authn.Principal{Subject: alice, Role: authz.Farmer})

func TestUseCases_FailuresAndGuards(t *testing.T) {
	type run func(d usecase.Deps) error
	fc := func(d usecase.Deps) error {
		raw, _ := json.Marshal(forecast(900, 0))
		return d.OnForecastUpdated(ctx, eventbus.Event{ID: "00000000-0000-7000-8000-000000000e01", Payload: raw})
	}
	sc := func(d usecase.Deps) error {
		raw, _ := json.Marshal(map[string]string{"owner_id": alice, "status": "completed", "disease_code": "rice_blast"})
		return d.OnScanCompleted(ctx, eventbus.Event{ID: "00000000-0000-7000-8000-000000000e02", Payload: raw})
	}
	list := func(d usecase.Deps) error {
		_, err := usecase.ListAlerts{Deps: d}.Execute(farmer, false, 10)
		return err
	}
	readAll := func(d usecase.Deps) error { _, err := usecase.MarkAllRead{Deps: d}.Execute(farmer); return err }
	getSub := func(d usecase.Deps) error { _, err := usecase.GetSubscription{Deps: d}.Execute(farmer); return err }
	putSub := func(d usecase.Deps) error {
		_, err := usecase.PutSubscription{Deps: d}.Execute(farmer, usecase.SubscriptionInput{Lat: 24.9, Lng: 89.4, Kinds: []string{"heavy_rain"}})
		return err
	}
	delSub := func(d usecase.Deps) error { return usecase.DeleteSubscription{Deps: d}.Execute(farmer) }
	cases := []struct {
		name  string
		fault harness.Fault
		run   run
	}{
		{"in cell", harness.Fault{Op: "query", Match: "WHERE cell_key", Err: boom}, fc},
		{"insert (forecast)", harness.Fault{Op: "exec", Match: "INSERT INTO alert_alerts", Err: boom}, fc},
		{"insert (scan)", harness.Fault{Op: "exec", Match: "INSERT INTO alert_alerts", Err: boom}, sc},
		{"list", harness.Fault{Op: "query", Match: "ORDER BY created_at", Err: boom}, list},
		{"unread count", harness.Fault{Op: "queryrow", Match: "count(*)", Err: boom}, list},
		{"read all", harness.Fault{Op: "exec", Match: "SET read_at = $2 WHERE user_id", Err: boom}, readAll},
		{"get sub", harness.Fault{Op: "query", Match: "WHERE user_id = $1", Err: boom}, getSub},
		{"put sub", harness.Fault{Op: "exec", Match: "INSERT INTO alert_subscriptions", Err: boom}, putSub},
		{"delete sub", harness.Fault{Op: "exec", Match: "DELETE FROM alert_subscriptions", Err: boom}, delSub},
	}
	shared := harness.DB(t) // one transaction: concurrent test transactions would block on alice's rows
	if err := putSub(deps(t, shared)); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if err := c.run(deps(t, harness.FaultyOver(shared, c.fault))); !errors.Is(err, boom) {
			t.Errorf("%s: want boom, got %v", c.name, err)
		}
	}
	d := deps(t, shared)
	if err := sc(d); err != nil {
		t.Fatal(err)
	}
	in, _ := usecase.ListAlerts{Deps: d}.Execute(farmer, false, 10)
	id := in.Alerts[0].ID
	for name, f := range map[string]harness.Fault{
		"get":       {Op: "query", Match: "WHERE id = $1", Err: boom},
		"mark read": {Op: "exec", Match: "COALESCE(read_at", Err: boom},
	} {
		if _, err := (usecase.MarkRead{Deps: deps(t, harness.FaultyOver(d.Alerts.(repository.Alerts).DB, f))}).Execute(farmer, id); !errors.Is(err, boom) {
			t.Errorf("%s: %v", name, err)
		}
	}
	bad := eventbus.Event{Payload: []byte("{")}
	if d.OnForecastUpdated(ctx, bad) == nil || d.OnScanCompleted(ctx, bad) == nil {
		t.Fatal("malformed payloads must fail (and be retried by the outbox)")
	}
	anon := ctx
	checks := map[string]error{}
	_, checks["list"] = usecase.ListAlerts{Deps: d}.Execute(anon, false, 1)
	_, checks["get"] = usecase.GetAlert{Deps: d}.Execute(anon, id)
	_, checks["read"] = usecase.MarkRead{Deps: d}.Execute(anon, id)
	_, checks["read all"] = usecase.MarkAllRead{Deps: d}.Execute(anon)
	_, checks["put"] = usecase.PutSubscription{Deps: d}.Execute(anon, usecase.SubscriptionInput{})
	_, checks["get sub"] = usecase.GetSubscription{Deps: d}.Execute(anon)
	checks["del sub"] = usecase.DeleteSubscription{Deps: d}.Execute(anon)
	for name, err := range checks {
		if !errors.Is(err, authn.ErrRequired) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, in := range []usecase.SubscriptionInput{{Lat: 91, Kinds: []string{"heat"}}, {Lng: 200, Kinds: []string{"heat"}}, {Kinds: nil}, {Kinds: []string{"flood"}}} {
		if _, err := (usecase.PutSubscription{Deps: d}).Execute(farmer, in); !errors.Is(err, domain.ErrInvalidSubscription) {
			t.Errorf("%+v accepted", in)
		}
	}
	if err := (alert.LogNotifier{Logger: logger.Discard()}).Notify(ctx, domain.Alert{}); err != nil || len(alert.Errors()) != 2 {
		t.Fatal()
	}
}

func TestHTTP_StorageFailures(t *testing.T) {
	cases := []struct {
		fault        harness.Fault
		method, path string
		body         any
	}{
		{harness.Fault{Op: "query", Match: "ORDER BY created_at", Err: boom}, "GET", "/v1/alerts", nil},
		{harness.Fault{Op: "exec", Match: "SET read_at = $2 WHERE user_id", Err: boom}, "POST", "/v1/alerts/read-all", nil},
		{harness.Fault{Op: "exec", Match: "INSERT INTO alert_subscriptions", Err: boom}, "PUT", "/v1/alerts/subscription",
			transport.SubscriptionRequest{Lat: 1, Lng: 1, Kinds: []string{"heat"}}},
		{harness.Fault{Op: "exec", Match: "DELETE FROM alert_subscriptions", Err: boom}, "DELETE", "/v1/alerts/subscription", nil},
	}
	for _, c := range cases {
		a := setup(t, harness.FaultyDB(t, c.fault), nil)
		if code := a.Do(c.method, c.path, c.body, a.Token(alice, authz.Farmer)).Code; code != 500 {
			t.Errorf("%s %s: %d", c.method, c.path, code)
		}
	}
}
