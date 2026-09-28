package domain

import (
	"math"
	"math/bits"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// Errors.
var (
	ErrFieldNotFound   = errs.NotFound("farm.field_not_found")
	ErrCropNotFound    = errs.NotFound("farm.crop_not_found")
	ErrInvalidField    = errs.Validation("farm.invalid_field")
	ErrInvalidLocation = errs.Validation("farm.invalid_location")
	ErrInvalidArea     = errs.Validation("farm.invalid_area")
)

// Errors lists the farm error catalogue.
func Errors() []*errs.Error {
	return []*errs.Error{ErrFieldNotFound, ErrCropNotFound, ErrInvalidField, ErrInvalidLocation, ErrInvalidArea}
}

// Coordinate is a WGS84 point stored as integer degrees × 1e7 (≈1 cm).
type Coordinate struct {
	LatE7 int64
	LngE7 int64
}

// NewCoordinate validates and rounds a latitude/longitude pair.
func NewCoordinate(lat, lng float64) (Coordinate, error) {
	if math.IsNaN(lat) || math.IsNaN(lng) || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return Coordinate{}, ErrInvalidLocation
	}
	return Coordinate{LatE7: int64(math.Round(lat * 1e7)), LngE7: int64(math.Round(lng * 1e7))}, nil
}

// Lat returns the latitude in degrees.
func (c Coordinate) Lat() float64 { return float64(c.LatE7) / 1e7 }

// Lng returns the longitude in degrees.
func (c Coordinate) Lng() float64 { return float64(c.LngE7) / 1e7 }

// Unit is a land-area unit.
type Unit string

// Units. Bigha follows the Bangladesh standard of 33 decimals.
const (
	Decimal Unit = "decimal"
	Bigha   Unit = "bigha"
	Acre    Unit = "acre"
	Hectare Unit = "hectare"
)

// Nano-square-metres (1e-9 m²) per unit — exact integers
// (1 acre = 4046.8564224 m², 1 decimal = 1/100 acre, 1 bigha = 33 decimals).
var nanoPer = map[Unit]uint64{
	Decimal: 40_468_564_224,
	Bigha:   33 * 40_468_564_224,
	Acre:    4_046_856_422_400,
	Hectare: 10_000_000_000_000,
}

// Units lists the supported units.
func Units() []Unit { return []Unit{Decimal, Bigha, Acre, Hectare} }

// MaxAreaNano bounds a single field (100 000 ha).
const MaxAreaNano uint64 = 1_000_000_000_000_000_000

// Area is a land area held in nano-square-metres, so conversions between
// decimal, bigha, acre and hectare are exact integer arithmetic (128-bit
// intermediates via math/bits; inputs are bounded so quotients fit).
type Area struct{ nano uint64 }

// mulDivRound returns round(a*b/c) with a 128-bit intermediate. Callers
// guarantee the quotient fits in 64 bits.
func mulDivRound(a, b, c uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	lo, carry := bits.Add64(lo, c/2, 0)
	q, _ := bits.Div64(hi+carry, lo, c)
	return q
}

// AreaFromMilli builds an area from thousandths of unit (2.5 bigha = 2500).
func AreaFromMilli(milli int64, unit Unit) (Area, error) {
	per, ok := nanoPer[unit]
	if !ok || milli <= 0 || uint64(milli) > mulDivRound(MaxAreaNano, 1000, per) {
		return Area{}, ErrInvalidArea
	}
	return Area{nano: mulDivRound(uint64(milli), per, 1000)}, nil
}

// AreaFromNano rebuilds a stored area.
func AreaFromNano(nano int64) Area { return Area{nano: uint64(max(nano, 0))} }

// Nano returns the canonical value.
func (a Area) Nano() int64 { return int64(a.nano) } //nolint:gosec // bounded by MaxAreaNano

// Milli expresses the area in thousandths of unit, rounded half up.
// Unknown units yield 0.
func (a Area) Milli(unit Unit) int64 {
	per, ok := nanoPer[unit]
	if !ok {
		return 0
	}
	return int64(mulDivRound(a.nano, 1000, per)) //nolint:gosec // bounded by MaxAreaNano
}

// Hectares returns the area in hectares as a fraction (for agronomic math that
// is itself approximate, e.g. yield estimates).
func (a Area) Hectares() float64 { return float64(a.nano) / float64(nanoPer[Hectare]) }

// Texture is a soil texture class.
type Texture string

// Textures.
const (
	Clay      Texture = "clay"
	ClayLoam  Texture = "clay_loam"
	Loam      Texture = "loam"
	SiltLoam  Texture = "silt_loam"
	SandyLoam Texture = "sandy_loam"
	Sandy     Texture = "sandy"
)

// Textures lists every texture.
func Textures() []Texture { return []Texture{Clay, ClayLoam, Loam, SiltLoam, SandyLoam, Sandy} }

// Irrigation levels.
const (
	IrrigationNone    = "none"
	IrrigationPartial = "partial"
	IrrigationFull    = "full"
)
