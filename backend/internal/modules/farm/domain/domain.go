// Package domain holds the farm model: fields, plots, soil and the crop catalogue.
package domain

import (
	"context"
	"time"
)

// SoilProfile is the latest soil test of a field. pH and organic matter are
// held ×10 as integers.
type SoilProfile struct {
	Texture          Texture
	PHx10            int
	NitrogenKgHa     int
	PhosphorusKgHa   int
	PotassiumKgHa    int
	OrganicMatterX10 int
	TestedOn         *time.Time
}

// Plot is a sub-division of a field with its current crop.
type Plot struct {
	ID        string
	FieldID   string
	Name      string
	Area      Area
	CropCode  string
	SownOn    *time.Time
	CreatedAt time.Time
}

// Field is a farmer's land parcel.
type Field struct {
	ID         string
	OwnerID    string
	Name       string
	Area       Area
	AreaUnit   Unit
	Location   Coordinate
	District   string
	Irrigation string
	Soil       *SoilProfile
	Plots      []Plot
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Fields is the field repository port.
type Fields interface {
	Create(ctx context.Context, f Field) error
	Get(ctx context.Context, id string) (Field, error)
	ListByOwner(ctx context.Context, ownerID string, limit int) ([]Field, error)
	Update(ctx context.Context, f Field) error
	Delete(ctx context.Context, id string) error
	PutSoil(ctx context.Context, fieldID string, s SoilProfile) error
	AddPlot(ctx context.Context, p Plot) error
}
