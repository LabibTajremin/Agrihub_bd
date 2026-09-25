// Package usecase implements the farm interactors. Every operation checks the
// principal's permission and, for a specific field, the ownership policy.
package usecase

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/tx"
)

// Event topics.
const (
	TopicFieldCreated = "farm.field.created"
	TopicFieldUpdated = "farm.field.updated"
	TopicFieldDeleted = "farm.field.deleted"
)

// MaxFieldsListed caps list responses.
const MaxFieldsListed = 200

// Deps are shared by the farm use cases.
type Deps struct {
	Fields  domain.Fields
	Tx      tx.Manager
	Clock   clock.Clock
	IDs     idgen.Generator
	Events  eventbus.Publisher
	Factory eventbus.Factory
}

// AreaInput is an area in thousandths of a unit.
type AreaInput struct {
	ValueMilli int64
	Unit       string
}

func (a AreaInput) build() (domain.Area, error) {
	return domain.AreaFromMilli(a.ValueMilli, domain.Unit(a.Unit))
}

// SoilInput is a soil test.
type SoilInput struct {
	Texture          string
	PH               float64
	NitrogenKgHa     int
	PhosphorusKgHa   int
	PotassiumKgHa    int
	OrganicMatterPct float64
	TestedOn         *time.Time
}

func (s SoilInput) build() (domain.SoilProfile, error) {
	if !slices.Contains(domain.Textures(), domain.Texture(s.Texture)) || s.PH < 3 || s.PH > 10 ||
		s.NitrogenKgHa < 0 || s.PhosphorusKgHa < 0 || s.PotassiumKgHa < 0 || s.OrganicMatterPct < 0 || s.OrganicMatterPct > 20 {
		return domain.SoilProfile{}, domain.ErrInvalidField.WithField("soil", "out of range")
	}
	return domain.SoilProfile{Texture: domain.Texture(s.Texture), PHx10: int(math.Round(s.PH * 10)),
		NitrogenKgHa: s.NitrogenKgHa, PhosphorusKgHa: s.PhosphorusKgHa, PotassiumKgHa: s.PotassiumKgHa,
		OrganicMatterX10: int(math.Round(s.OrganicMatterPct * 10)), TestedOn: s.TestedOn}, nil
}

func validIrrigation(s string) bool {
	return s == domain.IrrigationNone || s == domain.IrrigationPartial || s == domain.IrrigationFull
}

// FieldInput creates a field.
type FieldInput struct {
	Name       string
	Area       AreaInput
	Lat, Lng   float64
	District   string
	Irrigation string
	Soil       *SoilInput
}

func (d Deps) publish(ctx context.Context, topic, actor string, f domain.Field) error {
	e, err := d.Factory.New(ctx, topic, actor, map[string]string{"field_id": f.ID, "owner_id": f.OwnerID, "district": f.District})
	if err == nil {
		err = d.Events.Publish(ctx, e)
	}
	return err
}

// CreateField registers a field owned by the caller.
type CreateField struct{ Deps }

// Execute validates and stores the field (with an optional soil test).
func (uc CreateField) Execute(ctx context.Context, in FieldInput) (domain.Field, error) {
	p, err := authn.Require(ctx, authz.FieldCreate)
	if err != nil {
		return domain.Field{}, err
	}
	area, err := in.Area.build()
	if err != nil {
		return domain.Field{}, err
	}
	loc, err := domain.NewCoordinate(in.Lat, in.Lng)
	if err != nil {
		return domain.Field{}, err
	}
	if in.Name == "" || !validIrrigation(in.Irrigation) {
		return domain.Field{}, domain.ErrInvalidField
	}
	now := uc.Clock.Now()
	f := domain.Field{ID: uc.IDs.New(), OwnerID: p.Subject, Name: in.Name, Area: area, AreaUnit: domain.Unit(in.Area.Unit),
		Location: loc, District: in.District, Irrigation: in.Irrigation, CreatedAt: now, UpdatedAt: now}
	if in.Soil != nil {
		soil, err := in.Soil.build()
		if err != nil {
			return domain.Field{}, err
		}
		f.Soil = &soil
	}
	err = uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.Fields.Create(ctx, f); err != nil {
			return err
		}
		if f.Soil != nil {
			if err := uc.Fields.PutSoil(ctx, f.ID, *f.Soil); err != nil {
				return err
			}
		}
		return uc.publish(ctx, TopicFieldCreated, p.Subject, f)
	})
	return f, err
}

// load fetches a field and applies the ownership policy for (own, any).
func (d Deps) load(ctx context.Context, id string, own, anyPerm authz.Permission) (authn.Principal, domain.Field, error) {
	p, err := authn.Require(ctx, own)
	if err != nil {
		return p, domain.Field{}, err
	}
	f, err := d.Fields.Get(ctx, id)
	if err != nil {
		return p, domain.Field{}, err
	}
	return p, f, authz.Owned(p.Role, p.Subject, f.OwnerID, own, anyPerm)
}

// GetField returns one field (owner, or roles with field:read_any).
type GetField struct{ Deps }

// Execute loads the field.
func (uc GetField) Execute(ctx context.Context, id string) (domain.Field, error) {
	_, f, err := uc.load(ctx, id, authz.FieldRead, authz.FieldReadAny)
	if err != nil {
		return domain.Field{}, err
	}
	return f, nil
}

// ListFields lists the caller's fields, or another owner's with field:read_any.
type ListFields struct{ Deps }

// Execute lists fields of ownerID ("" = caller).
func (uc ListFields) Execute(ctx context.Context, ownerID string) ([]domain.Field, error) {
	p, err := authn.Require(ctx, authz.FieldRead)
	if err != nil {
		return nil, err
	}
	if ownerID == "" {
		ownerID = p.Subject
	}
	if err := authz.Owned(p.Role, p.Subject, ownerID, authz.FieldRead, authz.FieldReadAny); err != nil {
		return nil, err
	}
	return uc.Fields.ListByOwner(ctx, ownerID, MaxFieldsListed)
}

// FieldPatch holds optional changes.
type FieldPatch struct {
	Name       *string
	Area       *AreaInput
	Lat, Lng   *float64
	District   *string
	Irrigation *string
}

// UpdateField edits a field (owner, or admin).
type UpdateField struct{ Deps }

// Execute applies the patch.
func (uc UpdateField) Execute(ctx context.Context, id string, in FieldPatch) (domain.Field, error) {
	var out domain.Field
	err := uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		p, f, err := uc.load(ctx, id, authz.FieldWrite, authz.FieldWriteAny)
		if err != nil {
			return err
		}
		if err := apply(&f, in); err != nil {
			return err
		}
		f.UpdatedAt = uc.Clock.Now()
		if err := uc.Fields.Update(ctx, f); err != nil {
			return err
		}
		out = f
		return uc.publish(ctx, TopicFieldUpdated, p.Subject, f)
	})
	return out, err
}

func apply(f *domain.Field, in FieldPatch) error {
	if in.Name != nil {
		if *in.Name == "" {
			return domain.ErrInvalidField.WithField("name", "required")
		}
		f.Name = *in.Name
	}
	if in.Area != nil {
		a, err := in.Area.build()
		if err != nil {
			return err
		}
		f.Area, f.AreaUnit = a, domain.Unit(in.Area.Unit)
	}
	if in.Lat != nil || in.Lng != nil {
		lat, lng := f.Location.Lat(), f.Location.Lng()
		if in.Lat != nil {
			lat = *in.Lat
		}
		if in.Lng != nil {
			lng = *in.Lng
		}
		loc, err := domain.NewCoordinate(lat, lng)
		if err != nil {
			return err
		}
		f.Location = loc
	}
	if in.District != nil {
		f.District = *in.District
	}
	if in.Irrigation != nil {
		if !validIrrigation(*in.Irrigation) {
			return domain.ErrInvalidField.WithField("irrigation", "unknown")
		}
		f.Irrigation = *in.Irrigation
	}
	return nil
}

// DeleteField removes a field (owner, or admin).
type DeleteField struct{ Deps }

// Execute deletes the field.
func (uc DeleteField) Execute(ctx context.Context, id string) error {
	return uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		p, f, err := uc.load(ctx, id, authz.FieldWrite, authz.FieldWriteAny)
		if err != nil {
			return err
		}
		if err := uc.Fields.Delete(ctx, id); err != nil {
			return err
		}
		return uc.publish(ctx, TopicFieldDeleted, p.Subject, f)
	})
}

// PutSoil records a soil test.
type PutSoil struct{ Deps }

// Execute stores the profile and returns the updated field.
func (uc PutSoil) Execute(ctx context.Context, id string, in SoilInput) (domain.Field, error) {
	soil, err := in.build()
	if err != nil {
		return domain.Field{}, err
	}
	var out domain.Field
	err = uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		p, f, err := uc.load(ctx, id, authz.FieldWrite, authz.FieldWriteAny)
		if err != nil {
			return err
		}
		if err := uc.Fields.PutSoil(ctx, id, soil); err != nil {
			return err
		}
		f.Soil = &soil
		out = f
		return uc.publish(ctx, TopicFieldUpdated, p.Subject, f)
	})
	return out, err
}

// PlotInput adds a plot.
type PlotInput struct {
	Name     string
	Area     AreaInput
	CropCode string
	SownOn   *time.Time
}

// AddPlot adds a plot to a field; plots may not exceed the field's area.
type AddPlot struct{ Deps }

// Execute validates and stores the plot.
func (uc AddPlot) Execute(ctx context.Context, fieldID string, in PlotInput) (domain.Plot, error) {
	area, err := in.Area.build()
	if err != nil {
		return domain.Plot{}, err
	}
	if _, err := domain.FindCrop(in.CropCode); err != nil {
		return domain.Plot{}, err
	}
	var plot domain.Plot
	err = uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		_, f, err := uc.load(ctx, fieldID, authz.FieldWrite, authz.FieldWriteAny)
		if err != nil {
			return err
		}
		used := area.Nano()
		for _, p := range f.Plots {
			used += p.Area.Nano()
		}
		if used > f.Area.Nano() {
			return domain.ErrInvalidArea.WithField("area", "plots exceed the field")
		}
		plot = domain.Plot{ID: uc.IDs.New(), FieldID: fieldID, Name: in.Name, Area: area, CropCode: in.CropCode, SownOn: in.SownOn, CreatedAt: uc.Clock.Now()}
		return uc.Fields.AddPlot(ctx, plot)
	})
	return plot, err
}
