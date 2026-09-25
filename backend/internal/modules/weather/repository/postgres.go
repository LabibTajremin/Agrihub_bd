// Package repository implements the weather observation cache on Postgres.
package repository

import (
	"context"
	"encoding/json"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
)

// Observations implements domain.Observations.
type Observations struct{ DB *database.DB }

// Get loads the cached observation of a cell.
func (r Observations) Get(ctx context.Context, cell domain.Cell) (domain.Observation, error) {
	o := domain.Observation{Cell: cell}
	var raw []byte
	err := r.DB.Q(ctx).QueryRow(ctx, `SELECT forecast, fetched_at FROM weather_observations WHERE cell_key = $1`, cell.Key()).Scan(&raw, &o.FetchedAt)
	if err == nil {
		_ = json.Unmarshal(raw, &o.Forecast) // written by Put from the same type
		o.FetchedAt = o.FetchedAt.UTC()
	}
	return o, database.NotFound(err, domain.ErrNotCached)
}

// Put stores (or replaces) a cell's observation.
func (r Observations) Put(ctx context.Context, o domain.Observation) error {
	raw, _ := json.Marshal(o.Forecast)
	_, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO weather_observations (cell_key, lat_e1, lng_e1, forecast, fetched_at) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (cell_key) DO UPDATE SET forecast = EXCLUDED.forecast, fetched_at = EXCLUDED.fetched_at`,
		o.Cell.Key(), o.Cell.LatE1, o.Cell.LngE1, raw, o.FetchedAt)
	return err
}
