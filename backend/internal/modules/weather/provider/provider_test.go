package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/breaker"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
)

var ctx = context.Background()

func TestStub_Deterministic(t *testing.T) {
	s := Stub{Clock: clock.NewFake(time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))}
	a, _ := s.Forecast(ctx, 24.8, 89.4)
	b, _ := s.Forecast(ctx, 24.8, 89.4)
	if len(a.Days) != Days || a.Days[0].Date != "2026-07-01" || a.Days[6].Date != "2026-07-07" || a.Days[3] != b.Days[3] {
		t.Fatal(a)
	}
	for _, d := range a.Days {
		if d.TempMaxC10 < d.TempMinC10 || d.HumidityPct > 100 || d.RainMM10 < 0 {
			t.Fatalf("%+v", d)
		}
	}
}

const sample = `{"daily":{"time":["2026-07-01","2026-07-02"],"temperature_2m_max":[33.4,36.1],"temperature_2m_min":[26.2,27],
"precipitation_sum":[12.3,64.05],"relative_humidity_2m_mean":[88.4,79],"wind_speed_10m_max":[14.6,20.2]}}`

func TestOpenMeteo(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		switch r.URL.Query().Get("latitude") {
		case "1.0000":
			w.WriteHeader(502)
		case "2.0000":
			_, _ = w.Write([]byte("{"))
		case "3.0000":
			_, _ = w.Write([]byte(`{"daily":{"time":["a"],"temperature_2m_max":[]}}`))
		default:
			_, _ = w.Write([]byte(sample))
		}
	}))
	defer srv.Close()
	p := OpenMeteo{BaseURL: srv.URL, Client: srv.Client()}
	f, err := p.Forecast(ctx, 24.85, 89.37)
	if err != nil || len(f.Days) != 2 || f.Days[1].RainMM10 != 641 || f.Days[1].TempMaxC10 != 361 || f.Days[0].HumidityPct != 88 {
		t.Fatal(f, err)
	}
	if !strings.Contains(got, "forecast_days=7") || !strings.Contains(got, "precipitation_sum") {
		t.Fatal(got)
	}
	for _, lat := range []float64{1, 2, 3} {
		if _, err := p.Forecast(ctx, lat, 0); err == nil {
			t.Errorf("lat %v: expected error", lat)
		}
	}
	if _, err := (OpenMeteo{BaseURL: "http://127.0.0.1:1", Client: &http.Client{Timeout: time.Second}}).Forecast(ctx, 0, 0); err == nil {
		t.Fatal("transport error")
	}
	if _, err := (OpenMeteo{BaseURL: "://bad"}).Forecast(ctx, 0, 0); err == nil {
		t.Fatal("bad url")
	}
}

type failing struct{}

func (failing) Forecast(context.Context, float64, float64) (domain.Forecast, error) {
	return domain.Forecast{}, errors.New("down")
}

func TestGuarded_OpensAfterFailures(t *testing.T) {
	c := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	g := Guarded{Inner: failing{}, Breaker: breaker.New(breaker.Options{Threshold: 2, OpenTimeout: time.Minute, HalfOpenMax: 1, Clock: c})}
	_, _ = g.Forecast(ctx, 0, 0)
	_, _ = g.Forecast(ctx, 0, 0)
	if _, err := g.Forecast(ctx, 0, 0); !errors.Is(err, breaker.ErrOpen) {
		t.Fatal(err)
	}
	ok := Guarded{Inner: Stub{Clock: c}, Breaker: breaker.New(breaker.Options{Threshold: 1, OpenTimeout: time.Minute, HalfOpenMax: 1, Clock: c})}
	if f, err := ok.Forecast(ctx, 0, 0); err != nil || len(f.Days) != Days {
		t.Fatal(err)
	}
}
