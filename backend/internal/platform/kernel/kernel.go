// Package kernel bundles the platform services every module receives, so module
// Deps stay short and wiring stays explicit.
package kernel

import (
	"log/slog"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// Kernel is the shared platform handle.
type Kernel struct {
	DB        *database.DB
	Clock     clock.Clock
	IDs       idgen.Generator
	Validator *validator.Validator
	Logger    *slog.Logger
	// Events publishes transactionally (outbox); Bus is where consumers subscribe.
	Events eventbus.Publisher
	Bus    *eventbus.Local
}

// Factory returns an event factory using the kernel's clock and IDs.
func (k Kernel) Factory() eventbus.Factory { return eventbus.Factory{IDs: k.IDs, Clock: k.Clock} }
