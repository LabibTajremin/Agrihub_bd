// Package farm is the farm module (future farm-service): fields, plots, soil
// tests and the crop catalogue. Other modules read it through FieldView and
// CropView, never through its internals.
package farm

import (
	"context"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/kernel"
)

// Deps are the module's dependencies.
type Deps struct {
	Kernel kernel.Kernel
}

// Module is the assembled farm module.
type Module struct {
	handlers transport.Handlers
	fields   domain.Fields
}

// New wires the module.
func New(d Deps) (*Module, error) {
	k := d.Kernel
	repo := repository.Fields{DB: k.DB}
	ud := usecase.Deps{Fields: repo, Tx: k.DB, Clock: k.Clock, IDs: k.IDs, Events: k.Events, Factory: k.Factory()}
	return &Module{fields: repo, handlers: transport.Handlers{
		Create: usecase.CreateField{Deps: ud}, Get: usecase.GetField{Deps: ud}, List: usecase.ListFields{Deps: ud},
		Update: usecase.UpdateField{Deps: ud}, Delete: usecase.DeleteField{Deps: ud}, PutSoil: usecase.PutSoil{Deps: ud},
		AddPlot: usecase.AddPlot{Deps: ud}, V: k.Validator,
	}}, nil
}

// Routes returns the module's HTTP routes.
func (m *Module) Routes() []httpx.Route { return m.handlers.Routes() }

// Errors is the module's error catalogue.
func Errors() []*errs.Error { return domain.Errors() }

// SoilView is the published soil representation.
type SoilView struct {
	Texture      string
	PHx10        int
	NitrogenKgHa int
}

// FieldView is the published field representation for other modules. The
// caller is responsible for authorization (it receives OwnerID for that).
type FieldView struct {
	ID          string
	OwnerID     string
	District    string
	Irrigation  string
	Lat, Lng    float64
	AreaHectare float64
	Soil        *SoilView
	CropCodes   []string // crops currently on the field's plots
}

// Field returns the published view of a field.
func (m *Module) Field(ctx context.Context, id string) (FieldView, error) {
	f, err := m.fields.Get(ctx, id)
	if err != nil {
		return FieldView{}, err
	}
	v := FieldView{ID: f.ID, OwnerID: f.OwnerID, District: f.District, Irrigation: f.Irrigation,
		Lat: f.Location.Lat(), Lng: f.Location.Lng(), AreaHectare: f.Area.Hectares()}
	if f.Soil != nil {
		v.Soil = &SoilView{Texture: string(f.Soil.Texture), PHx10: f.Soil.PHx10, NitrogenKgHa: f.Soil.NitrogenKgHa}
	}
	for _, p := range f.Plots {
		v.CropCodes = append(v.CropCodes, p.CropCode)
	}
	return v, nil
}

// CropView is the published crop representation.
type CropView struct {
	Code              string
	Seasons           []string
	WaterNeedMM       int
	Textures          []string
	PHMinX10          int
	PHMaxX10          int
	NitrogenDeltaKgHa int
	YieldLow          int64
	YieldLikely       int64
	YieldHigh         int64
	PricePoishaPerKg  int64
	SeedAvailBP       int
	MarketDemandBP    int
	PestRiskBP        int
	CostPoishaPerHa   map[string]int64
}

// Crops returns the published catalogue.
func (m *Module) Crops() []CropView {
	cs := domain.Catalogue()
	out := make([]CropView, len(cs))
	for i, c := range cs {
		ts := make([]string, len(c.Textures))
		for j, t := range c.Textures {
			ts[j] = string(t)
		}
		out[i] = CropView{Code: c.Code, Seasons: c.Seasons, WaterNeedMM: c.WaterNeedMM, Textures: ts, PHMinX10: c.PHMinX10,
			PHMaxX10: c.PHMaxX10, NitrogenDeltaKgHa: c.NitrogenDeltaKgHa, YieldLow: c.YieldKgHa.Low, YieldLikely: c.YieldKgHa.Likely,
			YieldHigh: c.YieldKgHa.High, PricePoishaPerKg: c.PricePoishaPerKg, SeedAvailBP: c.SeedAvailBP,
			MarketDemandBP: c.MarketDemandBP, PestRiskBP: c.PestRiskBP, CostPoishaPerHa: c.CostPoishaPerHa}
	}
	return out
}
