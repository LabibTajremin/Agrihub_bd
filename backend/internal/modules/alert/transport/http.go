// Package transport exposes alerts and subscriptions over HTTP.
package transport

import (
	"net/http"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// Alert is one alert; title and body are dictionary keys with params.
type Alert struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Severity  string            `json:"severity"`
	TitleKey  string            `json:"title_key"`
	BodyKey   string            `json:"body_key"`
	Params    map[string]string `json:"params"`
	CreatedAt time.Time         `json:"created_at"`
	ReadAt    *time.Time        `json:"read_at,omitempty"`
}

// Inbox lists alerts.
type Inbox struct {
	Alerts      []Alert `json:"alerts"`
	UnreadCount int     `json:"unread_count"`
}

// InboxQuery filters alerts.
type InboxQuery struct {
	Unread bool `query:"unread"`
	Limit  int  `query:"limit"`
}

// ReadAll reports how many alerts were marked read.
type ReadAll struct {
	Updated int `json:"updated"`
}

// SubscriptionRequest sets the alert location and kinds.
type SubscriptionRequest struct {
	Lat   float64  `json:"lat" validate:"gte=-90,lte=90"`
	Lng   float64  `json:"lng" validate:"gte=-180,lte=180"`
	Kinds []string `json:"kinds" validate:"required,min=1,dive,oneof=heavy_rain heat blast_risk disease_followup"`
}

// Subscription is a stored subscription.
type Subscription struct {
	CellKey   string    `json:"cell_key"`
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	Kinds     []string  `json:"kinds"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Handlers groups the alert endpoints.
type Handlers struct {
	List      usecase.ListAlerts
	Get       usecase.GetAlert
	Read      usecase.MarkRead
	ReadAll   usecase.MarkAllRead
	PutSub    usecase.PutSubscription
	GetSub    usecase.GetSubscription
	DeleteSub usecase.DeleteSubscription
	V         *validator.Validator
}

// Routes declares the endpoints.
func (h Handlers) Routes() []httpx.Route {
	read, sub := string(authz.AlertRead), string(authz.SubscriptionEdit)
	return []httpx.Route{
		{Method: http.MethodGet, Path: "/v1/alerts", Permission: read, Tag: "alerts", Summary: "Alert inbox", Query: InboxQuery{}, Response: Inbox{}, Handler: h.list},
		{Method: http.MethodGet, Path: "/v1/alerts/{id}", Permission: read, Tag: "alerts", Summary: "One alert", Response: Alert{}, Handler: h.get},
		{Method: http.MethodPost, Path: "/v1/alerts/{id}/read", Permission: read, Tag: "alerts", Summary: "Mark read", Response: Alert{}, Handler: h.read},
		{Method: http.MethodPost, Path: "/v1/alerts/read-all", Permission: read, Tag: "alerts", Summary: "Mark all read", Response: ReadAll{}, Handler: h.readAll},
		{Method: http.MethodGet, Path: "/v1/alerts/subscription", Permission: read, Tag: "alerts", Summary: "Alert subscription", Response: Subscription{}, Handler: h.getSub},
		{Method: http.MethodPut, Path: "/v1/alerts/subscription", Permission: sub, Tag: "alerts", Summary: "Set alert location and kinds",
			Request: SubscriptionRequest{}, Response: Subscription{}, Handler: h.putSub},
		{Method: http.MethodDelete, Path: "/v1/alerts/subscription", Permission: sub, Tag: "alerts", Summary: "Stop area alerts", Status: http.StatusNoContent, Handler: h.deleteSub},
	}
}

// ToAlert renders an alert.
func ToAlert(a domain.Alert) Alert {
	return Alert{ID: a.ID, Kind: a.Kind, Severity: a.Severity, TitleKey: a.TitleKey, BodyKey: a.BodyKey, Params: a.Params, CreatedAt: a.CreatedAt, ReadAt: a.ReadAt}
}

func toSub(s domain.Subscription) Subscription {
	return Subscription{CellKey: s.CellKey, Lat: s.Lat, Lng: s.Lng, Kinds: s.Kinds, UpdatedAt: s.UpdatedAt}
}

func (h Handlers) list(w http.ResponseWriter, r *http.Request) error {
	limit, err := httpx.QueryInt(r, "limit", usecase.DefaultLimit, 1, usecase.MaxLimit)
	if err != nil {
		return err
	}
	in, err := h.List.Execute(r.Context(), r.URL.Query().Get("unread") == "true", int(limit))
	if err != nil {
		return err
	}
	out := Inbox{UnreadCount: in.Unread, Alerts: make([]Alert, len(in.Alerts))}
	for i, a := range in.Alerts {
		out.Alerts[i] = ToAlert(a)
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h Handlers) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	a, err := h.Get.Execute(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, ToAlert(a))
}

func (h Handlers) read(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	a, err := h.Read.Execute(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, ToAlert(a))
}

func (h Handlers) readAll(w http.ResponseWriter, r *http.Request) error {
	n, err := h.ReadAll.Execute(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, ReadAll{Updated: n})
}

func (h Handlers) getSub(w http.ResponseWriter, r *http.Request) error {
	s, err := h.GetSub.Execute(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, toSub(s))
}

func (h Handlers) putSub(w http.ResponseWriter, r *http.Request) error {
	var in SubscriptionRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	s, err := h.PutSub.Execute(r.Context(), usecase.SubscriptionInput{Lat: in.Lat, Lng: in.Lng, Kinds: in.Kinds})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, toSub(s))
}

func (h Handlers) deleteSub(w http.ResponseWriter, r *http.Request) error {
	if err := h.DeleteSub.Execute(r.Context()); err != nil {
		return err
	}
	return httpx.NoContent(w)
}
