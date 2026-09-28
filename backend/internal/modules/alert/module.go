// Package alert is the alert module (future alert-service): a rules engine
// driven by weather and diagnosis events, per-user inboxes and subscriptions.
// Push delivery is a port with a logging stub (no provider integration).
package alert

import (
	"context"
	"log/slog"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/kernel"
)

// Deps are the module's dependencies.
type Deps struct {
	Kernel   kernel.Kernel
	Notifier domain.Notifier // nil = logging stub
}

// Module is the assembled alert module.
type Module struct{ handlers transport.Handlers }

// New wires the module and subscribes its consumers to the event bus.
func New(d Deps) (*Module, error) {
	k := d.Kernel
	n := d.Notifier
	if n == nil {
		n = LogNotifier{Logger: k.Logger}
	}
	ud := usecase.Deps{Alerts: repository.Alerts{DB: k.DB}, Subscriptions: repository.Subscriptions{DB: k.DB}, Notifier: n,
		Clock: k.Clock, IDs: k.IDs, Logger: k.Logger}
	k.Bus.Subscribe(usecase.TopicForecastUpdated, ud.OnForecastUpdated)
	k.Bus.Subscribe(usecase.TopicScanCompleted, ud.OnScanCompleted)
	return &Module{handlers: transport.Handlers{List: usecase.ListAlerts{Deps: ud}, Get: usecase.GetAlert{Deps: ud}, Read: usecase.MarkRead{Deps: ud},
		ReadAll: usecase.MarkAllRead{Deps: ud}, PutSub: usecase.PutSubscription{Deps: ud}, GetSub: usecase.GetSubscription{Deps: ud},
		DeleteSub: usecase.DeleteSubscription{Deps: ud}, V: k.Validator}}, nil
}

// Routes returns the module's HTTP routes.
func (m *Module) Routes() []httpx.Route { return m.handlers.Routes() }

// Errors is the module's error catalogue.
func Errors() []*errs.Error { return domain.Errors() }

// LogNotifier is the push-notification stub: it logs instead of delivering.
type LogNotifier struct{ Logger *slog.Logger }

// Notify implements domain.Notifier.
func (l LogNotifier) Notify(ctx context.Context, a domain.Alert) error {
	l.Logger.InfoContext(ctx, "alert raised", "alert_id", a.ID, "kind", a.Kind, "severity", a.Severity)
	return nil
}
