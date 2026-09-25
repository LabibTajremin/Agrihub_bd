// Package repository implements the farm ports on Postgres.
package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
)

// Fields implements domain.Fields.
type Fields struct{ DB *database.DB }

const fieldSQL = `SELECT f.id, f.owner_id, f.name, f.area_nano, f.area_unit, f.lat_e7, f.lng_e7, f.district, f.irrigation,
	f.created_at, f.updated_at, s.texture, s.ph_x10, s.nitrogen_kg_ha, s.phosphorus_kg_ha, s.potassium_kg_ha,
	s.organic_matter_x10, s.tested_on
	FROM farm_fields f LEFT JOIN farm_soil_profiles s ON s.field_id = f.id`

func scanField(row pgx.CollectableRow) (domain.Field, error) {
	var f domain.Field
	var nano int64
	var unit string
	var texture *string
	var ph, n, p, k, om *int
	var tested *time.Time
	err := row.Scan(&f.ID, &f.OwnerID, &f.Name, &nano, &unit, &f.Location.LatE7, &f.Location.LngE7, &f.District,
		&f.Irrigation, &f.CreatedAt, &f.UpdatedAt, &texture, &ph, &n, &p, &k, &om, &tested)
	f.Area, f.AreaUnit = domain.AreaFromNano(nano), domain.Unit(unit)
	f.CreatedAt, f.UpdatedAt = f.CreatedAt.UTC(), f.UpdatedAt.UTC()
	if texture != nil {
		f.Soil = &domain.SoilProfile{Texture: domain.Texture(*texture), PHx10: *ph, NitrogenKgHa: *n, PhosphorusKgHa: *p,
			PotassiumKgHa: *k, OrganicMatterX10: *om, TestedOn: tested}
	}
	return f, err
}

func scanPlot(row pgx.CollectableRow) (domain.Plot, error) {
	var p domain.Plot
	var nano int64
	err := row.Scan(&p.ID, &p.FieldID, &p.Name, &nano, &p.CropCode, &p.SownOn, &p.CreatedAt)
	p.Area, p.CreatedAt = domain.AreaFromNano(nano), p.CreatedAt.UTC()
	return p, err
}

// Create inserts a field.
func (r Fields) Create(ctx context.Context, f domain.Field) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO farm_fields (id, owner_id, name, area_nano, area_unit, lat_e7, lng_e7, district, irrigation, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`, f.ID, f.OwnerID, f.Name, f.Area.Nano(), string(f.AreaUnit),
		f.Location.LatE7, f.Location.LngE7, f.District, f.Irrigation, f.CreatedAt, f.UpdatedAt)
	return err
}

// Get loads a field with its soil profile and plots.
func (r Fields) Get(ctx context.Context, id string) (domain.Field, error) {
	fs, err := database.Collect(ctx, r.DB.Q(ctx), scanField, fieldSQL+` WHERE f.id = $1`, id)
	if err == nil && len(fs) == 0 {
		err = domain.ErrFieldNotFound
	}
	if err != nil {
		return domain.Field{}, err
	}
	f := fs[0]
	f.Plots, err = database.Collect(ctx, r.DB.Q(ctx), scanPlot,
		`SELECT id, field_id, name, area_nano, crop_code, sown_on, created_at FROM farm_plots WHERE field_id = $1 ORDER BY created_at, id`, id)
	return f, err
}

// ListByOwner lists an owner's fields (without plots), oldest first.
func (r Fields) ListByOwner(ctx context.Context, ownerID string, limit int) ([]domain.Field, error) {
	return database.Collect(ctx, r.DB.Q(ctx), scanField, fieldSQL+` WHERE f.owner_id = $1 ORDER BY f.created_at, f.id LIMIT $2`, ownerID, limit)
}

// Update persists mutable columns.
func (r Fields) Update(ctx context.Context, f domain.Field) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `UPDATE farm_fields SET name = $2, area_nano = $3, area_unit = $4, lat_e7 = $5, lng_e7 = $6,
		district = $7, irrigation = $8, updated_at = $9 WHERE id = $1`, f.ID, f.Name, f.Area.Nano(), string(f.AreaUnit),
		f.Location.LatE7, f.Location.LngE7, f.District, f.Irrigation, f.UpdatedAt)
	return err
}

// Delete removes a field (plots and soil cascade).
func (r Fields) Delete(ctx context.Context, id string) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `DELETE FROM farm_fields WHERE id = $1`, id)
	return err
}

// PutSoil replaces the soil profile.
func (r Fields) PutSoil(ctx context.Context, fieldID string, s domain.SoilProfile) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO farm_soil_profiles (field_id, texture, ph_x10, nitrogen_kg_ha, phosphorus_kg_ha, potassium_kg_ha, organic_matter_x10, tested_on)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (field_id) DO UPDATE SET texture = EXCLUDED.texture, ph_x10 = EXCLUDED.ph_x10, nitrogen_kg_ha = EXCLUDED.nitrogen_kg_ha,
		phosphorus_kg_ha = EXCLUDED.phosphorus_kg_ha, potassium_kg_ha = EXCLUDED.potassium_kg_ha,
		organic_matter_x10 = EXCLUDED.organic_matter_x10, tested_on = EXCLUDED.tested_on`,
		fieldID, string(s.Texture), s.PHx10, s.NitrogenKgHa, s.PhosphorusKgHa, s.PotassiumKgHa, s.OrganicMatterX10, s.TestedOn)
	return err
}

// AddPlot inserts a plot.
func (r Fields) AddPlot(ctx context.Context, p domain.Plot) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO farm_plots (id, field_id, name, area_nano, crop_code, sown_on, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, p.ID, p.FieldID, p.Name, p.Area.Nano(), p.CropCode, p.SownOn, p.CreatedAt)
	return err
}
