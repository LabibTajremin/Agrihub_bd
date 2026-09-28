// Package usecase implements weather reads: cache-first forecasts with
// staleness metadata, provider refresh through the circuit breaker, and the
// seasonal outlook used by the Crop Advisor.
package usecase

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/tx"
)

// TopicForecastUpdated is published whenever a fresh forecast is fetched.
const TopicForecastUpdated = "weather.forecast.updated"

// Deps are shared by the weather use cases.
type Deps struct {
	Observations domain.Observations
	Provider     domain.Provider
	Tx           tx.Manager
	Clock        clock.Clock
	Events       eventbus.Publisher
	Factory      eventbus.Factory
	StaleAfter   time.Duration
}

// ForecastPayload is the event payload (consumers decode their own copy).
type ForecastPayload struct {
	CellKey string       `json:"cell_key"`
	Lat     float64      `json:"lat"`
	Lng     float64      `json:"lng"`
	Days    []domain.Day `json:"days"`
}

// Fetch returns the forecast for a point: cached if younger than a third of
// stale_after, otherwise refreshed from the provider; if the provider fails,
// the cached copy is served with its age (flagged stale), never hidden.
func (d Deps) Fetch(ctx context.Context, lat, lng float64) (domain.Report, error) {
	cell, err := domain.CellOf(lat, lng)
	if err != nil {
		return domain.Report{}, err
	}
	now := d.Clock.Now()
	cached, err := d.Observations.Get(ctx, cell)
	have := err == nil
	if err != nil && !errors.Is(err, domain.ErrNotCached) {
		return domain.Report{}, err
	}
	if have && now.Sub(cached.FetchedAt) < d.StaleAfter/3 {
		return domain.NewReport(cached, now, d.StaleAfter), nil
	}
	f, perr := d.Provider.Forecast(ctx, cell.Lat(), cell.Lng())
	if perr != nil {
		if have {
			return domain.NewReport(cached, now, d.StaleAfter), nil
		}
		return domain.Report{}, domain.ErrUnavailable.Wrap(perr)
	}
	obs := domain.Observation{Cell: cell, Forecast: f, FetchedAt: now}
	err = d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := d.Observations.Put(ctx, obs); err != nil {
			return err
		}
		e, err := d.Factory.New(ctx, TopicForecastUpdated, "", ForecastPayload{CellKey: cell.Key(), Lat: cell.Lat(), Lng: cell.Lng(), Days: f.Days})
		if err == nil {
			err = d.Events.Publish(ctx, e)
		}
		return err
	})
	return domain.NewReport(obs, now, d.StaleAfter), err
}

// GetForecast serves the daily forecast (weather:read).
type GetForecast struct{ Deps }

// Execute returns the report for a point.
func (uc GetForecast) Execute(ctx context.Context, lat, lng float64) (domain.Report, error) {
	if _, err := authn.Require(ctx, authz.WeatherRead); err != nil {
		return domain.Report{}, err
	}
	return uc.Fetch(ctx, lat, lng)
}

// Outlook is the seasonal rainfall outlook.
type Outlook struct {
	Season     string // the current season
	RainMM     map[string]int
	AgeSeconds int64
	Stale      bool
}

// seasonAt mirrors the advisory calendar: Jul–Oct aman, Mar–Jun aus, else boro.
func seasonAt(m time.Month) string {
	switch {
	case m >= time.July && m <= time.October:
		return "aman"
	case m >= time.March && m <= time.June:
		return "aus"
	default:
		return "boro"
	}
}

// SeasonalOutlook starts from regional climatology and scales the current
// season by this week's rain anomaly (clamped to ±30%). Without any forecast
// it returns climatology flagged stale.
func (d Deps) SeasonalOutlook(ctx context.Context, lat, lng float64) (Outlook, error) {
	if _, err := domain.CellOf(lat, lng); err != nil {
		return Outlook{}, err
	}
	season := seasonAt(d.Clock.Now().Month())
	rain := domain.Climatology(lat, lng)
	rep, err := d.Fetch(ctx, lat, lng)
	if err != nil {
		return Outlook{Season: season, RainMM: rain, Stale: true}, nil
	}
	var week int
	for _, day := range rep.Forecast.Days {
		week += day.RainMM10
	}
	normalWeek := float64(rain[season]) * 7 / 122 // seasons are ~4 months
	factor := math.Max(0.7, math.Min(1.3, float64(week)/10/math.Max(normalWeek, 1)))
	rain[season] = int(math.Round(float64(rain[season]) * factor))
	return Outlook{Season: season, RainMM: rain, AgeSeconds: rep.AgeSeconds, Stale: rep.Stale}, nil
}

// GetOutlook serves the seasonal outlook (weather:read).
type GetOutlook struct{ Deps }

// Execute returns the outlook for a point.
func (uc GetOutlook) Execute(ctx context.Context, lat, lng float64) (Outlook, error) {
	if _, err := authn.Require(ctx, authz.WeatherRead); err != nil {
		return Outlook{}, err
	}
	return uc.SeasonalOutlook(ctx, lat, lng)
}
