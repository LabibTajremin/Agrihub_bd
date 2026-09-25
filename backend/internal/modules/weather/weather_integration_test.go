//go:build integration

package weather_test

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/provider"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/config"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/test/apitest"
	"github.com/labibtajremin/agrihub_bd/backend/test/harness"
)

var (
	ctx  = context.Background()
	boom = errors.New("boom")
)

type fakeProvider struct {
	mu    sync.Mutex
	calls int
	err   error
	rain  int
}

func (f *fakeProvider) Forecast(_ context.Context, lat, lng float64) (domain.Forecast, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return domain.Forecast{}, f.err
	}
	days := make([]domain.Day, 7)
	for i := range days {
		days[i] = domain.Day{Date: "2026-03-0" + string(rune('1'+i)), TempMinC10: 220, TempMaxC10: 330, RainMM10: f.rain, HumidityPct: 70, WindKph: 10}
	}
	return domain.Forecast{Days: days}, nil
}

var cfg = config.Weather{StaleAfter: 3 * time.Hour}

func setup(t *testing.T, db *database.DB, p domain.Provider) (*apitest.API, *weather.Module) {
	t.Helper()
	a := apitest.New(t, db)
	m, err := weather.New(weather.Deps{Kernel: a.Kernel, Provider: p, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	a.Mount(m.Routes())
	return a, m
}

const at = "/v1/weather?lat=24.85&lng=89.37"

func TestForecast_CacheRefreshAndStaleness(t *testing.T) {
	p := &fakeProvider{rain: 120}
	a, _ := setup(t, harness.DB(t), p)
	tok := a.Token("00000000-0000-7000-8000-0000000000a1", authz.Guest)
	var f transport.Forecast
	a.Do("GET", at, nil, tok).Expect(t, 200).Decode(t, &f)
	if f.CellKey != "24.9:89.4" || f.DataAgeSeconds != 0 || f.Stale || len(f.Days) != 7 || f.Days[0].RainMM != 12 || f.Days[0].TempMaxC != 33 {
		t.Fatalf("%+v", f)
	}
	a.Clock.Advance(59 * time.Minute)
	a.Do("GET", at, nil, tok).Expect(t, 200).Decode(t, &f)
	if p.calls != 1 || f.DataAgeSeconds != 59*60 {
		t.Fatal("served from cache within stale_after/3", p.calls)
	}
	a.Clock.Advance(2 * time.Minute)
	a.Do("GET", at, nil, tok).Expect(t, 200).Decode(t, &f)
	if p.calls != 2 || f.DataAgeSeconds != 0 {
		t.Fatal("refreshed after the window", p.calls)
	}
	p.err = boom
	a.Clock.Advance(4 * time.Hour)
	a.Do("GET", at, nil, tok).Expect(t, 200).Decode(t, &f)
	if !f.Stale || f.DataAgeSeconds != 4*3600 {
		t.Fatalf("provider down: stale data is flagged, not hidden: %+v", f)
	}
	if res, _ := a.Relay.Flush(ctx); res.Dispatched != 2 {
		t.Fatal("one event per fresh fetch", res)
	}
	if c := a.Do("GET", "/v1/weather?lat=10&lng=10", nil, tok).Expect(t, 503).ErrorCode(); c != "weather.unavailable" {
		t.Fatal("no cache and no provider", c)
	}
	a.Do("GET", "/v1/weather?lat=100&lng=1", nil, tok).Expect(t, 400)
	a.Do("GET", "/v1/weather?lat=1&lng=nope", nil, tok).Expect(t, 400)
	a.Do("GET", at, nil, "").Expect(t, 401)
}

func TestSeasonalOutlook(t *testing.T) {
	p := &fakeProvider{rain: 1000} // 100 mm/day → very wet week → capped at +30%
	a, m := setup(t, harness.DB(t), p)
	tok := a.Token("00000000-0000-7000-8000-0000000000a1", authz.Farmer)
	var o transport.Outlook
	a.Do("GET", "/v1/weather/seasonal?lat=23.8&lng=90.4", nil, tok).Expect(t, 200).Decode(t, &o)
	if o.CurrentSeason != "aus" || o.RainMM["aus"] != 780 || o.RainMM["aman"] != 1600 || o.Stale {
		t.Fatalf("March is aus; anomaly scales only the current season: %+v", o)
	}
	p.err = boom
	v, err := m.Outlook(ctx, 10, 10)
	if err != nil || !v.Stale || v.RainMM["boro"] == 0 {
		t.Fatal("no forecast: climatology flagged stale", v, err)
	}
	a.Do("GET", "/v1/weather/seasonal?lat=91&lng=0", nil, tok).Expect(t, 400)
	if _, err := m.Outlook(ctx, 91, 0); !errors.Is(err, domain.ErrInvalidLocation) {
		t.Fatal(err)
	}
	a.Do("GET", "/v1/weather/seasonal?lat=1", nil, tok).Expect(t, 400)
	a.Do("GET", "/v1/weather/seasonal?lat=1&lng=1", nil, "").Expect(t, 401)
}

func TestSeasonAtCoversEveryMonth(t *testing.T) {
	p := &fakeProvider{rain: 0}
	a, m := setup(t, harness.DB(t), p)
	want := map[time.Month]string{time.January: "boro", time.April: "aus", time.August: "aman", time.December: "boro"}
	for month, season := range want {
		a.Clock.Set(time.Date(2026, month, 15, 0, 0, 0, 0, time.UTC))
		v, _ := m.Outlook(ctx, 23.8, 90.4)
		base := domain.Climatology(23.8, 90.4)[season]
		if v.RainMM[season] != int(float64(base)*0.7+0.5) {
			t.Errorf("%s: dry week scales %s down by 30%%: %d", month, season, v.RainMM[season])
		}
	}
}

func TestStorageFailures(t *testing.T) {
	cases := map[string]harness.Fault{
		"get":     {Op: "queryrow", Match: "FROM weather_observations", Err: boom},
		"put":     {Op: "exec", Match: "INSERT INTO weather_observations", Err: boom},
		"publish": {Op: "exec", Match: "INSERT INTO outbox", Err: boom},
	}
	for name, f := range cases {
		a, _ := setup(t, harness.FaultyDB(t, f), &fakeProvider{})
		if code := a.Do("GET", at, nil, a.Token("u", authz.Guest)).Code; code != 500 {
			t.Errorf("%s: %d", name, code)
		}
	}
}

func TestNewProvider(t *testing.T) {
	a := apitest.New(t, nil)
	c := config.Weather{Provider: "http", BaseURL: "http://127.0.0.1:1", HTTPTimeout: time.Second, BreakerFailureThreshold: 1,
		BreakerOpenTimeout: time.Minute, BreakerHalfOpenMaxCalls: 1}
	g, ok := weather.NewProvider(c, a.Kernel).(provider.Guarded)
	if !ok {
		t.Fatal("providers are wrapped in the breaker")
	}
	if _, ok := g.Inner.(provider.OpenMeteo); !ok {
		t.Fatal("http provider")
	}
	c.Provider = "stub"
	if _, ok := weather.NewProvider(c, a.Kernel).(provider.Guarded).Inner.(provider.Stub); !ok {
		t.Fatal("stub provider")
	}
	m, err := weather.New(weather.Deps{Kernel: apitest.New(t, harness.DB(t)).Kernel, Config: c})
	if err != nil || m == nil || len(weather.Errors()) != 2 {
		t.Fatal(err)
	}
}

func TestUseCases_Guards(t *testing.T) {
	a := apitest.New(t, harness.DB(t))
	d := usecase.Deps{Observations: repository.Observations{DB: a.Kernel.DB}, Provider: &fakeProvider{}, Tx: a.Kernel.DB, Clock: a.Clock,
		Events: a.Kernel.Events, Factory: a.Kernel.Factory(), StaleAfter: time.Hour}
	if _, err := (usecase.GetForecast{Deps: d}).Execute(ctx, 1, 1); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	if _, err := (usecase.GetOutlook{Deps: d}).Execute(ctx, 1, 1); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	guest := authn.WithPrincipal(ctx, authn.Principal{Subject: "g", Role: authz.Guest})
	if _, err := (usecase.GetForecast{Deps: d}).Execute(guest, math.NaN(), 1); !errors.Is(err, domain.ErrInvalidLocation) {
		t.Fatal(err)
	}
}
