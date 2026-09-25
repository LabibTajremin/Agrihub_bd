// Package diagnosis is the Plant Doctor module (future diagnosis-service):
// scans, AI analysis through the aiadapter port, treatment plans and offline sync.
package diagnosis

import (
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/cache"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/kernel"
)

// Deps are the module's dependencies.
type Deps struct {
	Kernel        kernel.Kernel
	Engine        aiadapter.DiagnosisEngine
	Media         domain.Media
	MinConfidence float64
	CacheSize     int
	MaxImageBytes int64
}

// Module is the assembled diagnosis module.
type Module struct{ handlers transport.Handlers }

// New wires the module.
func New(d Deps) (*Module, error) {
	lru, err := cache.NewLRU[uint64, domain.Diagnosis](d.CacheSize)
	if err != nil {
		return nil, err
	}
	k := d.Kernel
	ud := usecase.Deps{Scans: repository.Scans{DB: k.DB}, Media: d.Media, Engine: d.Engine, Cache: lru, Tx: k.DB, Clock: k.Clock,
		IDs: k.IDs, Events: k.Events, Factory: k.Factory(), MinConfidence: d.MinConfidence, MaxImageBytes: d.MaxImageBytes}
	create, annotate := usecase.CreateScan{Deps: ud}, usecase.Annotate{Deps: ud}
	return &Module{handlers: transport.Handlers{Create: create, Retry: usecase.Retry{Deps: ud}, Get: usecase.GetScan{Deps: ud},
		List: usecase.ListScans{Deps: ud}, Annotate: annotate, Sync: usecase.Sync{Deps: ud, Create: create, Annotate: annotate}, V: k.Validator}}, nil
}

// Routes returns the module's HTTP routes.
func (m *Module) Routes() []httpx.Route { return m.handlers.Routes() }

// Errors is the module's error catalogue.
func Errors() []*errs.Error { return domain.Errors() }
