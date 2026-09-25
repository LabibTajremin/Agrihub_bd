// Package domain holds the advisory algorithms as pure functions: weighted
// multi-criteria crop scoring, heap-based top-N ranking, integer yield/ROI and
// DAG rotation planning.
package domain

import (
	"context"
	"math"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// Errors.
var (
	ErrNoCandidates  = errs.NotFound("advisory.no_candidates")
	ErrRotationCycle = errs.Internal("advisory.rotation_cycle")
	ErrInvalidInput  = errs.Validation("advisory.invalid_input")
)

// Errors lists the advisory error catalogue.
func Errors() []*errs.Error { return []*errs.Error{ErrNoCandidates, ErrRotationCycle, ErrInvalidInput} }

// Field is what advisory needs to know about a field.
type Field struct {
	ID           string
	OwnerID      string
	Lat, Lng     float64
	AreaNano     int64 // 1e-9 m²
	Texture      string
	PHx10        int // 0 = unknown
	NitrogenKgHa int // 0 = unknown
	Irrigation   string
	CurrentCrop  string
}

// Yield is kg per hectare (catalogue) or kg for a field (estimate).
type Yield struct{ Low, Likely, High int64 }

// Crop is what advisory needs to know about a crop.
type Crop struct {
	Code              string
	Seasons           []string
	WaterNeedMM       int
	Textures          []string
	PHMinX10          int
	PHMaxX10          int
	NitrogenDeltaKgHa int
	YieldKgHa         Yield
	PricePoishaPerKg  int64
	SeedAvailBP       int
	MarketDemandBP    int
	PestRiskBP        int
	CostPoishaPerHa   map[string]int64
}

// Outlook is the seasonal rainfall forecast at a field.
type Outlook struct {
	RainMM     map[string]int
	AgeSeconds int64
	Stale      bool
}

// DefaultRainMM is the climatological fallback when no forecast is available.
var DefaultRainMM = map[string]int{"aman": 1600, "boro": 150, "aus": 600}

// Ports onto other modules (implemented by adapters in the composition root).
type (
	FieldSource interface {
		Field(ctx context.Context, id string) (Field, error)
	}
	CropSource interface {
		Crops() []Crop
	}
	WeatherSource interface {
		Outlook(ctx context.Context, lat, lng float64) (Outlook, error)
	}
)

// Weights are the MCDA criterion weights; they must sum to 1.
type Weights struct{ Soil, Water, Pest, Market, Seed float64 }

// WeightTolerance is the allowed float error of the weight sum.
const WeightTolerance = 1e-9

// Validate asserts every weight is in [0,1] and they sum to 1.
func (w Weights) Validate() error {
	for _, x := range []float64{w.Soil, w.Water, w.Pest, w.Market, w.Seed} {
		if x < 0 || x > 1 || math.IsNaN(x) {
			return ErrInvalidInput.WithField("weights", "each weight must be in [0,1]")
		}
	}
	if math.Abs(w.Soil+w.Water+w.Pest+w.Market+w.Seed-1) > WeightTolerance {
		return ErrInvalidInput.WithField("weights", "must sum to 1.0")
	}
	return nil
}
