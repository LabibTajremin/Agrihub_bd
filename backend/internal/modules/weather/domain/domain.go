// Package domain holds the weather model: daily forecasts on a 0.1° grid,
// observations with staleness metadata, and a seasonal rainfall outlook.
package domain

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// Errors.
var (
	ErrUnavailable     = errs.Unavailable("weather.unavailable")
	ErrInvalidLocation = errs.Validation("weather.invalid_location")
	ErrNotCached       = errs.NotFound("weather.not_cached")
)

// Errors lists the weather error catalogue (ErrNotCached is internal).
func Errors() []*errs.Error { return []*errs.Error{ErrUnavailable, ErrInvalidLocation} }

// Day is one forecast day. Temperatures and rain are tenths (°C×10, mm×10).
type Day struct {
	Date        string `json:"date"` // YYYY-MM-DD
	TempMinC10  int    `json:"temp_min_c10"`
	TempMaxC10  int    `json:"temp_max_c10"`
	RainMM10    int    `json:"rain_mm10"`
	HumidityPct int    `json:"humidity_pct"`
	WindKph     int    `json:"wind_kph"`
}

// Forecast is the provider's daily forecast for a cell.
type Forecast struct {
	Days []Day `json:"days"`
}

// Cell is a 0.1° grid square; forecasts are cached per cell.
type Cell struct {
	LatE1 int
	LngE1 int
}

// CellOf validates a point and returns its cell.
func CellOf(lat, lng float64) (Cell, error) {
	if math.IsNaN(lat) || math.IsNaN(lng) || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return Cell{}, ErrInvalidLocation
	}
	return Cell{LatE1: int(math.Round(lat * 10)), LngE1: int(math.Round(lng * 10))}, nil
}

// Key is the cache key, e.g. "24.8:89.4".
func (c Cell) Key() string { return fmt.Sprintf("%.1f:%.1f", float64(c.LatE1)/10, float64(c.LngE1)/10) }

// Lat returns the cell centre latitude.
func (c Cell) Lat() float64 { return float64(c.LatE1) / 10 }

// Lng returns the cell centre longitude.
func (c Cell) Lng() float64 { return float64(c.LngE1) / 10 }

// Observation is a cached forecast with its fetch time.
type Observation struct {
	Cell      Cell
	Forecast  Forecast
	FetchedAt time.Time
}

// Report is what clients receive: the forecast plus how old it is. Stale data
// is flagged, never hidden.
type Report struct {
	Observation
	AgeSeconds int64
	Stale      bool
}

// NewReport computes age and staleness at now.
func NewReport(o Observation, now time.Time, staleAfter time.Duration) Report {
	age := max(now.Sub(o.FetchedAt), 0)
	return Report{Observation: o, AgeSeconds: int64(age / time.Second), Stale: age > staleAfter}
}

// Climatology is the normal seasonal rainfall (mm) for Bangladesh, adjusted
// by latitude: the north-west is drier, the north-east and coast wetter.
func Climatology(lat, lng float64) map[string]int {
	factor := 1.0
	switch {
	case lng < 89.5 && lat > 24:
		factor = 0.85 // north-west (Rajshahi, Rangpur)
	case lng > 91:
		factor = 1.25 // north-east (Sylhet) and Chattogram hills
	case lat < 22.8:
		factor = 1.15 // coastal south
	}
	scale := func(mm int) int { return int(math.Round(float64(mm) * factor)) }
	return map[string]int{"aman": scale(1600), "boro": scale(150), "aus": scale(600)}
}

// Provider fetches a fresh forecast for a point.
type Provider interface {
	Forecast(ctx context.Context, lat, lng float64) (Forecast, error)
}

// Observations is the observation cache port.
type Observations interface {
	Get(ctx context.Context, cell Cell) (Observation, error)
	Put(ctx context.Context, o Observation) error
}
