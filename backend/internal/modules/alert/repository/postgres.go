// Package repository implements the alert ports on Postgres.
package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
)

// Alerts implements domain.Alerts.
type Alerts struct{ DB *database.DB }

const alertCols = `SELECT id, user_id, kind, severity, title_key, body_key, params, source_event_id, created_at, read_at FROM alert_alerts`

func scanAlert(row pgx.CollectableRow) (domain.Alert, error) {
	var a domain.Alert
	var params []byte
	err := row.Scan(&a.ID, &a.UserID, &a.Kind, &a.Severity, &a.TitleKey, &a.BodyKey, &params, &a.SourceEventID, &a.CreatedAt, &a.ReadAt)
	_ = json.Unmarshal(params, &a.Params) // written by Insert from a map[string]string
	a.CreatedAt = a.CreatedAt.UTC()
	return a, err
}

// Insert stores an alert once per (user, source event, kind).
func (r Alerts) Insert(ctx context.Context, a domain.Alert) (bool, error) {
	params, _ := json.Marshal(a.Params)
	tag, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO alert_alerts (id, user_id, kind, severity, title_key, body_key, params, source_event_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (user_id, source_event_id, kind) DO NOTHING`,
		a.ID, a.UserID, a.Kind, a.Severity, a.TitleKey, a.BodyKey, params, a.SourceEventID, a.CreatedAt)
	return tag.RowsAffected() == 1, err
}

// Get loads an alert.
func (r Alerts) Get(ctx context.Context, id string) (domain.Alert, error) {
	as, err := database.Collect(ctx, r.DB.Q(ctx), scanAlert, alertCols+` WHERE id = $1`, id)
	if err == nil && len(as) == 0 {
		err = domain.ErrNotFound
	}
	if err != nil {
		return domain.Alert{}, err
	}
	return as[0], nil
}

// List returns a user's alerts, newest first.
func (r Alerts) List(ctx context.Context, userID string, unreadOnly bool, limit int) ([]domain.Alert, error) {
	return database.Collect(ctx, r.DB.Q(ctx), scanAlert, alertCols+` WHERE user_id = $1 AND (NOT $2 OR read_at IS NULL)
		ORDER BY created_at DESC, id DESC LIMIT $3`, userID, unreadOnly, limit)
}

// MarkRead marks one alert read (the first read time is kept).
func (r Alerts) MarkRead(ctx context.Context, id string, at time.Time) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `UPDATE alert_alerts SET read_at = COALESCE(read_at, $2) WHERE id = $1`, id, at)
	return err
}

// MarkAllRead marks every unread alert of a user read.
func (r Alerts) MarkAllRead(ctx context.Context, userID string, at time.Time) (int, error) {
	tag, err := r.DB.Q(ctx).Exec(ctx, `UPDATE alert_alerts SET read_at = $2 WHERE user_id = $1 AND read_at IS NULL`, userID, at)
	return int(tag.RowsAffected()), err
}

// UnreadCount counts a user's unread alerts.
func (r Alerts) UnreadCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.DB.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM alert_alerts WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

// Subscriptions implements domain.Subscriptions.
type Subscriptions struct{ DB *database.DB }

const subCols = `SELECT user_id, cell_key, lat, lng, kinds, updated_at FROM alert_subscriptions`

func scanSub(row pgx.CollectableRow) (domain.Subscription, error) {
	var s domain.Subscription
	err := row.Scan(&s.UserID, &s.CellKey, &s.Lat, &s.Lng, &s.Kinds, &s.UpdatedAt)
	s.UpdatedAt = s.UpdatedAt.UTC()
	return s, err
}

// Put upserts a user's subscription.
func (r Subscriptions) Put(ctx context.Context, s domain.Subscription) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO alert_subscriptions (user_id, cell_key, lat, lng, kinds, updated_at) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id) DO UPDATE SET cell_key = EXCLUDED.cell_key, lat = EXCLUDED.lat, lng = EXCLUDED.lng, kinds = EXCLUDED.kinds,
		updated_at = EXCLUDED.updated_at`, s.UserID, s.CellKey, s.Lat, s.Lng, s.Kinds, s.UpdatedAt)
	return err
}

// Get loads a user's subscription.
func (r Subscriptions) Get(ctx context.Context, userID string) (domain.Subscription, error) {
	ss, err := database.Collect(ctx, r.DB.Q(ctx), scanSub, subCols+` WHERE user_id = $1`, userID)
	if err == nil && len(ss) == 0 {
		err = domain.ErrNotFound
	}
	if err != nil {
		return domain.Subscription{}, err
	}
	return ss[0], nil
}

// Delete removes a user's subscription.
func (r Subscriptions) Delete(ctx context.Context, userID string) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `DELETE FROM alert_subscriptions WHERE user_id = $1`, userID)
	return err
}

// InCell lists subscriptions in a cell.
func (r Subscriptions) InCell(ctx context.Context, cellKey string) ([]domain.Subscription, error) {
	return database.Collect(ctx, r.DB.Q(ctx), scanSub, subCols+` WHERE cell_key = $1 ORDER BY user_id`, cellKey)
}
