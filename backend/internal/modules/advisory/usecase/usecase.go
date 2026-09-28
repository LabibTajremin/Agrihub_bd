// Package usecase implements the Crop Advisor interactors. Advisory is
// stateless: it reads fields, crops and weather through ports and computes.
package usecase

import (
	"context"
	"slices"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
)

// DefaultNitrogenKgHa is assumed when a field has no soil test.
const DefaultNitrogenKgHa = 100

// MaxRotationSeasons bounds rotation plans.
const MaxRotationSeasons = 6

// Deps are shared by the advisory use cases.
type Deps struct {
	Fields   domain.FieldSource
	Crops    domain.CropSource
	Weather  domain.WeatherSource
	Narrator aiadapter.AdvisoryNarrator
	Weights  domain.Weights
	TopN     int
	Clock    clock.Clock
}

// load authorises the caller for the field (owner or field:read_any).
func (d Deps) load(ctx context.Context, fieldID string) (domain.Field, error) {
	p, err := authn.Require(ctx, authz.AdvisoryRead)
	if err != nil {
		return domain.Field{}, err
	}
	f, err := d.Fields.Field(ctx, fieldID)
	if err != nil {
		return domain.Field{}, err
	}
	return f, authz.Owned(p.Role, p.Subject, f.OwnerID, authz.FieldRead, authz.FieldReadAny)
}

// outlook degrades to climatology (flagged stale) when weather is unavailable.
func (d Deps) outlook(ctx context.Context, f domain.Field) domain.Outlook {
	o, err := d.Weather.Outlook(ctx, f.Lat, f.Lng)
	if err != nil {
		return domain.Outlook{RainMM: domain.DefaultRainMM, Stale: true}
	}
	return o
}

func (d Deps) season(s string) (string, error) {
	if s == "" {
		return domain.SeasonAt(int(d.Clock.Now().Month())), nil
	}
	if !slices.Contains([]string{"aman", "boro", "aus"}, s) {
		return "", domain.ErrInvalidInput.WithField("season", "aman, boro or aus")
	}
	return s, nil
}

// Item is one ranked crop with its yield estimate for the field.
type Item struct {
	domain.Scored
	Yield domain.Yield
}

// Recommendation is the ranked list for a field and season.
type Recommendation struct {
	Season  string
	Outlook domain.Outlook
	Items   []Item
}

// Recommend ranks crops for a field and season.
type Recommend struct{ Deps }

// Execute returns the top-N crops.
func (uc Recommend) Execute(ctx context.Context, fieldID, season string) (Recommendation, error) {
	f, err := uc.load(ctx, fieldID)
	if err != nil {
		return Recommendation{}, err
	}
	season, err = uc.season(season)
	if err != nil {
		return Recommendation{}, err
	}
	o := uc.outlook(ctx, f)
	top := domain.Rank(f, uc.Crops.Crops(), season, o.RainMM[season], uc.Weights, uc.TopN)
	if len(top) == 0 {
		return Recommendation{}, domain.ErrNoCandidates
	}
	items := make([]Item, len(top))
	for i, s := range top {
		items[i] = Item{Scored: s, Yield: domain.EstimateYield(s.Crop.YieldKgHa, f.AreaNano)}
	}
	return Recommendation{Season: season, Outlook: o, Items: items}, nil
}

func (d Deps) crop(code string) (domain.Crop, error) {
	for _, c := range d.Crops.Crops() {
		if c.Code == code {
			return c, nil
		}
	}
	return domain.Crop{}, domain.ErrInvalidInput.WithField("crop_code", "unknown")
}

// Detail is one crop evaluated for a field.
type Detail struct {
	Season  string
	Outlook domain.Outlook
	Scored  domain.Scored
	Yield   domain.Yield
	Costs   map[string]int64
	ROI     domain.ROI
}

// CropDetail evaluates one crop on a field (score breakdown, yield, default ROI).
type CropDetail struct{ Deps }

// Execute evaluates crop code on field fieldID.
func (uc CropDetail) Execute(ctx context.Context, fieldID, code, season string) (Detail, error) {
	f, err := uc.load(ctx, fieldID)
	if err != nil {
		return Detail{}, err
	}
	c, err := uc.crop(code)
	if err != nil {
		return Detail{}, err
	}
	if season == "" {
		season = c.Seasons[0]
	}
	if season, err = uc.season(season); err != nil {
		return Detail{}, err
	}
	o := uc.outlook(ctx, f)
	cr := domain.Normalise(f, c, o.RainMM[season])
	y := domain.EstimateYield(c.YieldKgHa, f.AreaNano)
	costs := domain.DefaultCosts(c.CostPoishaPerHa, f.AreaNano)
	return Detail{Season: season, Outlook: o, Scored: domain.Scored{Crop: c, Criteria: cr, Score: domain.Score(cr, uc.Weights)},
		Yield: y, Costs: costs, ROI: domain.EstimateROI(y, c.PricePoishaPerKg, costs)}, nil
}

// ROIResult is a priced estimate.
type ROIResult struct {
	Yield domain.Yield
	Costs map[string]int64
	ROI   domain.ROI
}

// EstimateROI prices a crop on a field with default or supplied costs.
type EstimateROI struct{ Deps }

// Execute estimates ROI; costs == nil uses the catalogue costs for the area.
func (uc EstimateROI) Execute(ctx context.Context, fieldID, code string, costs map[string]int64) (ROIResult, error) {
	f, err := uc.load(ctx, fieldID)
	if err != nil {
		return ROIResult{}, err
	}
	c, err := uc.crop(code)
	if err != nil {
		return ROIResult{}, err
	}
	for k, v := range costs {
		if v < 0 {
			return ROIResult{}, domain.ErrInvalidInput.WithField("costs."+k, "must be non-negative")
		}
	}
	if costs == nil {
		costs = domain.DefaultCosts(c.CostPoishaPerHa, f.AreaNano)
	}
	y := domain.EstimateYield(c.YieldKgHa, f.AreaNano)
	return ROIResult{Yield: y, Costs: costs, ROI: domain.EstimateROI(y, c.PricePoishaPerKg, costs)}, nil
}

// PlanRotation builds a nitrogen-balanced rotation for a field.
type PlanRotation struct{ Deps }

// Execute plans n seasons starting at start ("" = current season).
func (uc PlanRotation) Execute(ctx context.Context, fieldID, start string, n int) (domain.Rotation, error) {
	f, err := uc.load(ctx, fieldID)
	if err != nil {
		return domain.Rotation{}, err
	}
	if start, err = uc.season(start); err != nil {
		return domain.Rotation{}, err
	}
	if n < 1 || n > MaxRotationSeasons {
		return domain.Rotation{}, domain.ErrInvalidInput.WithField("seasons", "1 to 6")
	}
	seasons := domain.SeasonsFrom(start, n)
	o := uc.outlook(ctx, f)
	crops := uc.Crops.Crops()
	byCode := make(map[string]domain.Crop, len(crops))
	scores := map[string]float64{}
	for _, c := range crops {
		byCode[c.Code] = c
		scores[c.Code] = domain.Score(domain.Normalise(f, c, o.RainMM[c.Seasons[0]]), uc.Weights)
	}
	nitrogen := f.NitrogenKgHa
	if nitrogen == 0 {
		nitrogen = DefaultNitrogenKgHa
	}
	g := domain.BuildRotationGraph(f.CurrentCrop, seasons, crops)
	return domain.PlanRotation(g, "0:"+f.CurrentCrop, seasons, byCode, scores, nitrogen)
}

// Narrate explains a chart through the AdvisoryNarrator port.
type Narrate struct{ Deps }

// Execute returns the narration key (text + pre-recorded voice).
func (uc Narrate) Execute(ctx context.Context, in aiadapter.NarrateInput) (aiadapter.NarrateResult, error) {
	if _, err := authn.Require(ctx, authz.AdvisoryRead); err != nil {
		return aiadapter.NarrateResult{}, err
	}
	return uc.Narrator.Narrate(ctx, in)
}
