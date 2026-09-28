// Package provider holds the forecast provider adapters: a deterministic stub
// (default, offline), an Open-Meteo HTTP client, and the circuit-breaker
// decorator that keeps a failing provider from taking the API down.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/breaker"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
)

// Days is the forecast horizon.
const Days = 7

// Stub produces a plausible, deterministic forecast from location and date.
type Stub struct{ Clock clock.Clock }

// Forecast implements domain.Provider.
func (s Stub) Forecast(_ context.Context, lat, lng float64) (domain.Forecast, error) {
	start := s.Clock.Now().Truncate(24 * time.Hour)
	seed := int(math.Abs(lat*1000+lng*100)) + start.YearDay()
	days := make([]domain.Day, Days)
	for i := range days {
		v := (seed + i*37) % 100
		days[i] = domain.Day{
			Date:       start.AddDate(0, 0, i).Format(time.DateOnly),
			TempMinC10: 180 + v%60, TempMaxC10: 280 + v%90,
			RainMM10: (v * v) % 700, HumidityPct: 60 + v%35, WindKph: 5 + v%20,
		}
	}
	return domain.Forecast{Days: days}, nil
}

// OpenMeteo calls an Open-Meteo compatible /v1/forecast endpoint.
type OpenMeteo struct {
	BaseURL string
	Client  *http.Client
}

type omResponse struct {
	Daily struct {
		Time     []string  `json:"time"`
		TempMax  []float64 `json:"temperature_2m_max"`
		TempMin  []float64 `json:"temperature_2m_min"`
		Rain     []float64 `json:"precipitation_sum"`
		Humidity []float64 `json:"relative_humidity_2m_mean"`
		Wind     []float64 `json:"wind_speed_10m_max"`
	} `json:"daily"`
}

func tenths(x float64) int { return int(math.Round(x * 10)) }

// Forecast implements domain.Provider.
func (p OpenMeteo) Forecast(ctx context.Context, lat, lng float64) (domain.Forecast, error) {
	q := url.Values{
		"latitude": {strconv.FormatFloat(lat, 'f', 4, 64)}, "longitude": {strconv.FormatFloat(lng, 'f', 4, 64)},
		"daily":         {"temperature_2m_max,temperature_2m_min,precipitation_sum,relative_humidity_2m_mean,wind_speed_10m_max"},
		"forecast_days": {strconv.Itoa(Days)}, "timezone": {"auto"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.BaseURL+"/v1/forecast?"+q.Encode(), nil)
	if err != nil {
		return domain.Forecast{}, err
	}
	res, err := p.Client.Do(req)
	if err != nil {
		return domain.Forecast{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return domain.Forecast{}, fmt.Errorf("open-meteo: status %d", res.StatusCode)
	}
	var body omResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return domain.Forecast{}, err
	}
	d := body.Daily
	n := len(d.Time)
	if n == 0 || len(d.TempMax) != n || len(d.TempMin) != n || len(d.Rain) != n || len(d.Humidity) != n || len(d.Wind) != n {
		return domain.Forecast{}, fmt.Errorf("open-meteo: inconsistent daily arrays")
	}
	out := domain.Forecast{Days: make([]domain.Day, n)}
	for i := range n {
		out.Days[i] = domain.Day{Date: d.Time[i], TempMinC10: tenths(d.TempMin[i]), TempMaxC10: tenths(d.TempMax[i]),
			RainMM10: tenths(d.Rain[i]), HumidityPct: int(math.Round(d.Humidity[i])), WindKph: int(math.Round(d.Wind[i]))}
	}
	return out, nil
}

// Guarded wraps a provider with a circuit breaker.
type Guarded struct {
	Inner   domain.Provider
	Breaker *breaker.Breaker
}

// Forecast implements domain.Provider.
func (g Guarded) Forecast(ctx context.Context, lat, lng float64) (domain.Forecast, error) {
	var f domain.Forecast
	err := g.Breaker.Do(ctx, func(ctx context.Context) error {
		var err error
		f, err = g.Inner.Forecast(ctx, lat, lng)
		return err
	})
	return f, err
}
