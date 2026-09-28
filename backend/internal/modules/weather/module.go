// Package weather is the weather module (future weather-service): cached
// forecasts with staleness metadata behind a circuit-broken provider.
package weather

import (
	"context"
	"net/http"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/provider"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/breaker"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/config"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/kernel"
)

// Deps are the module's dependencies.
type Deps struct {
	Kernel   kernel.Kernel
	Provider domain.Provider // nil = built from Config
	Config   config.Weather
}

// Module is the assembled weather module.
type Module struct {
	handlers transport.Handlers
	deps     usecase.Deps
}

// NewProvider builds the configured provider wrapped in a circuit breaker.
func NewProvider(cfg config.Weather, k kernel.Kernel) domain.Provider {
	var inner domain.Provider = provider.Stub{Clock: k.Clock}
	if cfg.Provider == "http" {
		inner = provider.OpenMeteo{BaseURL: cfg.BaseURL, Client: &http.Client{Timeout: cfg.HTTPTimeout}}
	}
	return provider.Guarded{Inner: inner, Breaker: breaker.New(breaker.Options{Threshold: cfg.BreakerFailureThreshold,
		OpenTimeout: cfg.BreakerOpenTimeout, HalfOpenMax: cfg.BreakerHalfOpenMaxCalls, Clock: k.Clock})}
}

// New wires the module.
func New(d Deps) (*Module, error) {
	k := d.Kernel
	p := d.Provider
	if p == nil {
		p = NewProvider(d.Config, k)
	}
	ud := usecase.Deps{Observations: repository.Observations{DB: k.DB}, Provider: p, Tx: k.DB, Clock: k.Clock,
		Events: k.Events, Factory: k.Factory(), StaleAfter: d.Config.StaleAfter}
	return &Module{deps: ud, handlers: transport.Handlers{Forecast: usecase.GetForecast{Deps: ud}, Outlook: usecase.GetOutlook{Deps: ud}}}, nil
}

// Routes returns the module's HTTP routes.
func (m *Module) Routes() []httpx.Route { return m.handlers.Routes() }

// Errors is the module's error catalogue.
func Errors() []*errs.Error { return domain.Errors() }

// OutlookView is the published seasonal outlook for other modules.
type OutlookView struct {
	RainMM     map[string]int
	AgeSeconds int64
	Stale      bool
}

// Outlook returns the seasonal outlook for a point (callers authorize).
func (m *Module) Outlook(ctx context.Context, lat, lng float64) (OutlookView, error) {
	o, err := m.deps.SeasonalOutlook(ctx, lat, lng)
	return OutlookView{RainMM: o.RainMM, AgeSeconds: o.AgeSeconds, Stale: o.Stale}, err
}
