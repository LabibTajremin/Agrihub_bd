// Package advisory is the Crop Advisor module (future advisory-service):
// crop suitability, yield and ROI estimates, rotation plans and chart narration.
// It owns no tables; it reads other modules through its ports.
package advisory

import (
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/kernel"
)

// Deps are the module's dependencies.
type Deps struct {
	Kernel   kernel.Kernel
	Fields   domain.FieldSource
	Crops    domain.CropSource
	Weather  domain.WeatherSource
	Narrator aiadapter.AdvisoryNarrator
	Weights  domain.Weights
	TopN     int
}

// Module is the assembled advisory module.
type Module struct{ handlers transport.Handlers }

// New validates the weights (they must sum to 1.0) and wires the module.
func New(d Deps) (*Module, error) {
	if err := d.Weights.Validate(); err != nil {
		return nil, err
	}
	ud := usecase.Deps{Fields: d.Fields, Crops: d.Crops, Weather: d.Weather, Narrator: d.Narrator, Weights: d.Weights, TopN: d.TopN, Clock: d.Kernel.Clock}
	return &Module{handlers: transport.Handlers{Recommend: usecase.Recommend{Deps: ud}, Detail: usecase.CropDetail{Deps: ud},
		ROI: usecase.EstimateROI{Deps: ud}, Rotation: usecase.PlanRotation{Deps: ud}, Narrate: usecase.Narrate{Deps: ud}, V: d.Kernel.Validator}}, nil
}

// Routes returns the module's HTTP routes.
func (m *Module) Routes() []httpx.Route { return m.handlers.Routes() }

// Errors is the module's error catalogue.
func Errors() []*errs.Error { return domain.Errors() }
