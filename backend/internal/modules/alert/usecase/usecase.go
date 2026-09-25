// Package usecase implements alerts: event consumers that run the rules engine,
// and the user-facing read/unread and subscription operations.
package usecase

import (
	"context"
	"log/slog"
	"slices"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
)

// Consumed topics (other modules' events, referenced by name only).
const (
	TopicForecastUpdated = "weather.forecast.updated"
	TopicScanCompleted   = "diagnosis.scan.completed"
)

// Deps are shared by the alert use cases.
type Deps struct {
	Alerts        domain.Alerts
	Subscriptions domain.Subscriptions
	Notifier      domain.Notifier
	Clock         clock.Clock
	IDs           idgen.Generator
	Logger        *slog.Logger
}

// raise stores a candidate for a user (idempotent per event) and notifies on first insert.
func (d Deps) raise(ctx context.Context, c domain.Candidate, userID, eventID string) error {
	a := c.Build(d.IDs.New(), userID, eventID, d.Clock.Now())
	inserted, err := d.Alerts.Insert(ctx, a)
	if err != nil || !inserted {
		return err
	}
	if err := d.Notifier.Notify(ctx, a); err != nil {
		d.Logger.WarnContext(ctx, "alert notification failed", "alert_id", a.ID, "error", err)
	}
	return nil
}

type forecastEvent struct {
	CellKey string       `json:"cell_key"`
	Days    []domain.Day `json:"days"`
}

// OnForecastUpdated alerts every subscriber of the cell for each triggered,
// subscribed kind. Safe to redeliver (at-least-once).
func (d Deps) OnForecastUpdated(ctx context.Context, e eventbus.Event) error {
	var p forecastEvent
	if err := e.Decode(&p); err != nil {
		return err
	}
	cands := domain.EvaluateForecast(p.Days)
	if len(cands) == 0 {
		return nil
	}
	subs, err := d.Subscriptions.InCell(ctx, p.CellKey)
	if err != nil {
		return err
	}
	for _, s := range subs {
		for _, c := range cands {
			if !slices.Contains(s.Kinds, c.Kind) {
				continue
			}
			if err := d.raise(ctx, c, s.UserID, e.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

type scanEvent struct {
	OwnerID     string `json:"owner_id"`
	Status      string `json:"status"`
	DiseaseCode string `json:"disease_code"`
}

// OnScanCompleted reminds the owner to follow up on a diagnosed disease.
func (d Deps) OnScanCompleted(ctx context.Context, e eventbus.Event) error {
	var p scanEvent
	if err := e.Decode(&p); err != nil {
		return err
	}
	for _, c := range domain.EvaluateScan(p.Status, p.DiseaseCode) {
		if err := d.raise(ctx, c, p.OwnerID, e.ID); err != nil {
			return err
		}
	}
	return nil
}

// Inbox is a page of alerts plus the unread count.
type Inbox struct {
	Alerts []domain.Alert
	Unread int
}

// List limits.
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// ListAlerts returns the caller's alerts.
type ListAlerts struct{ Deps }

// Execute lists alerts, optionally only unread ones.
func (uc ListAlerts) Execute(ctx context.Context, unreadOnly bool, limit int) (Inbox, error) {
	p, err := authn.Require(ctx, authz.AlertRead)
	if err != nil {
		return Inbox{}, err
	}
	as, err := uc.Alerts.List(ctx, p.Subject, unreadOnly, limit)
	if err != nil {
		return Inbox{}, err
	}
	n, err := uc.Alerts.UnreadCount(ctx, p.Subject)
	return Inbox{Alerts: as, Unread: n}, err
}

// own loads an alert of the caller; other users' alerts read as not found.
func (d Deps) own(ctx context.Context, id string) (domain.Alert, error) {
	p, err := authn.Require(ctx, authz.AlertRead)
	if err != nil {
		return domain.Alert{}, err
	}
	a, err := d.Alerts.Get(ctx, id)
	if err == nil && a.UserID != p.Subject {
		err = domain.ErrNotFound
	}
	return a, err
}

// GetAlert returns one of the caller's alerts.
type GetAlert struct{ Deps }

// Execute loads alert id.
func (uc GetAlert) Execute(ctx context.Context, id string) (domain.Alert, error) {
	return uc.own(ctx, id)
}

// MarkRead marks one alert read.
type MarkRead struct{ Deps }

// Execute marks alert id read and returns it.
func (uc MarkRead) Execute(ctx context.Context, id string) (domain.Alert, error) {
	a, err := uc.own(ctx, id)
	if err != nil {
		return domain.Alert{}, err
	}
	now := uc.Clock.Now()
	if a.ReadAt == nil {
		a.ReadAt = &now
	}
	return a, uc.Alerts.MarkRead(ctx, id, now)
}

// MarkAllRead marks every unread alert read.
type MarkAllRead struct{ Deps }

// Execute returns how many alerts changed.
func (uc MarkAllRead) Execute(ctx context.Context) (int, error) {
	p, err := authn.Require(ctx, authz.AlertRead)
	if err != nil {
		return 0, err
	}
	return uc.Alerts.MarkAllRead(ctx, p.Subject, uc.Clock.Now())
}

// SubscriptionInput sets the alert location and kinds.
type SubscriptionInput struct {
	Lat, Lng float64
	Kinds    []string
}

// PutSubscription creates or replaces the caller's subscription.
type PutSubscription struct{ Deps }

// Execute validates and stores the subscription.
func (uc PutSubscription) Execute(ctx context.Context, in SubscriptionInput) (domain.Subscription, error) {
	p, err := authn.Require(ctx, authz.SubscriptionEdit)
	if err != nil {
		return domain.Subscription{}, err
	}
	if in.Lat < -90 || in.Lat > 90 || in.Lng < -180 || in.Lng > 180 || len(in.Kinds) == 0 {
		return domain.Subscription{}, domain.ErrInvalidSubscription
	}
	for _, k := range in.Kinds {
		if !slices.Contains(domain.Kinds(), k) {
			return domain.Subscription{}, domain.ErrInvalidSubscription.WithField("kinds", k)
		}
	}
	s := domain.Subscription{UserID: p.Subject, CellKey: domain.CellKey(in.Lat, in.Lng), Lat: in.Lat, Lng: in.Lng,
		Kinds: in.Kinds, UpdatedAt: uc.Clock.Now()}
	return s, uc.Subscriptions.Put(ctx, s)
}

// GetSubscription returns the caller's subscription.
type GetSubscription struct{ Deps }

// Execute loads it.
func (uc GetSubscription) Execute(ctx context.Context) (domain.Subscription, error) {
	p, err := authn.Require(ctx, authz.AlertRead)
	if err != nil {
		return domain.Subscription{}, err
	}
	return uc.Subscriptions.Get(ctx, p.Subject)
}

// DeleteSubscription removes the caller's subscription.
type DeleteSubscription struct{ Deps }

// Execute deletes it.
func (uc DeleteSubscription) Execute(ctx context.Context) error {
	p, err := authn.Require(ctx, authz.SubscriptionEdit)
	if err != nil {
		return err
	}
	return uc.Subscriptions.Delete(ctx, p.Subject)
}
