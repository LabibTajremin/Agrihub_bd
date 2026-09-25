// Package localization is the i18n module (future i18n-service): languages,
// key→value dictionaries per language and pre-recorded voice manifests.
package localization

import (
	"context"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/seed"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/kernel"
)

// Deps are the module's dependencies.
type Deps struct {
	Kernel      kernel.Kernel
	CacheMaxAge time.Duration
}

// Module is the assembled localization module.
type Module struct {
	handlers transport.Handlers
	seed     usecase.Seed
}

// New wires the module.
func New(d Deps) (*Module, error) {
	k := d.Kernel
	ud := usecase.Deps{Store: repository.Store{DB: k.DB}, Catalog: &domain.Catalog{}, Tx: k.DB, Clock: k.Clock, Events: k.Events, Factory: k.Factory()}
	return &Module{
		handlers: transport.Handlers{List: usecase.ListLanguages{Deps: ud}, Get: usecase.GetDictionary{Deps: ud},
			Voice: usecase.GetVoiceManifest{Deps: ud}, Upsert: usecase.UpsertEntries{Deps: ud},
			UpsertVoice: usecase.UpsertVoice{Deps: ud}, V: k.Validator, MaxAge: d.CacheMaxAge},
		seed: usecase.Seed{Deps: ud},
	}, nil
}

// Routes returns the module's HTTP routes.
func (m *Module) Routes() []httpx.Route { return m.handlers.Routes() }

// SeedBundled loads the embedded dictionaries into the store.
func (m *Module) SeedBundled(ctx context.Context) (map[string]int, error) {
	return m.seed.From(ctx, seed.Load)
}

// Errors is the module's error catalogue.
func Errors() []*errs.Error { return domain.Errors() }
