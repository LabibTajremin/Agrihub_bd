// Package transport exposes fields and the crop catalogue over HTTP.
package transport

import (
	"net/http"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// AreaIn is an area in thousandths of a unit (2.5 bigha → 2500).
type AreaIn struct {
	ValueMilli int64  `json:"value_milli" validate:"gt=0"`
	Unit       string `json:"unit" validate:"oneof=decimal bigha acre hectare"`
}

// Area is returned in every unit.
type Area struct {
	Unit         string `json:"unit"`
	ValueMilli   int64  `json:"value_milli"`
	DecimalMilli int64  `json:"decimal_milli"`
	BighaMilli   int64  `json:"bigha_milli"`
	AcreMilli    int64  `json:"acre_milli"`
	HectareMilli int64  `json:"hectare_milli"`
}

// Location is a WGS84 point.
type Location struct {
	Lat float64 `json:"lat" validate:"gte=-90,lte=90"`
	Lng float64 `json:"lng" validate:"gte=-180,lte=180"`
}

// Soil is a soil test.
type Soil struct {
	Texture          string     `json:"texture" validate:"oneof=clay clay_loam loam silt_loam sandy_loam sandy"`
	PH               float64    `json:"ph" validate:"gte=3,lte=10"`
	NitrogenKgHa     int        `json:"nitrogen_kg_ha" validate:"gte=0,lte=2000"`
	PhosphorusKgHa   int        `json:"phosphorus_kg_ha" validate:"gte=0,lte=2000"`
	PotassiumKgHa    int        `json:"potassium_kg_ha" validate:"gte=0,lte=2000"`
	OrganicMatterPct float64    `json:"organic_matter_pct" validate:"gte=0,lte=20"`
	TestedOn         *time.Time `json:"tested_on,omitempty"`
}

// FieldRequest creates a field.
type FieldRequest struct {
	Name       string   `json:"name" validate:"required,max=80"`
	Area       AreaIn   `json:"area"`
	Location   Location `json:"location"`
	District   string   `json:"district" validate:"max=60"`
	Irrigation string   `json:"irrigation" validate:"oneof=none partial full"`
	Soil       *Soil    `json:"soil,omitempty"`
}

// FieldPatchRequest edits a field.
type FieldPatchRequest struct {
	Name       *string   `json:"name" validate:"omitempty,max=80"`
	Area       *AreaIn   `json:"area"`
	Location   *Location `json:"location"`
	District   *string   `json:"district" validate:"omitempty,max=60"`
	Irrigation *string   `json:"irrigation" validate:"omitempty,oneof=none partial full"`
}

// PlotRequest adds a plot.
type PlotRequest struct {
	Name     string     `json:"name" validate:"required,max=80"`
	Area     AreaIn     `json:"area"`
	CropCode string     `json:"crop_code" validate:"required,max=40"`
	SownOn   *time.Time `json:"sown_on,omitempty"`
}

// Plot is a sub-division of a field.
type Plot struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Area     Area       `json:"area"`
	CropCode string     `json:"crop_code"`
	SownOn   *time.Time `json:"sown_on,omitempty"`
}

// Field is the field representation.
type Field struct {
	ID         string    `json:"id"`
	OwnerID    string    `json:"owner_id"`
	Name       string    `json:"name"`
	Area       Area      `json:"area"`
	Location   Location  `json:"location"`
	District   string    `json:"district"`
	Irrigation string    `json:"irrigation"`
	Soil       *Soil     `json:"soil,omitempty"`
	Plots      []Plot    `json:"plots"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// FieldList lists fields.
type FieldList struct {
	Fields []Field `json:"fields"`
}

// OwnerQuery selects another owner (field:read_any).
type OwnerQuery struct {
	OwnerID string `query:"owner_id"`
}

// Yield is kg per hectare.
type Yield struct {
	Low    int64 `json:"low"`
	Likely int64 `json:"likely"`
	High   int64 `json:"high"`
}

// Crop is a catalogue entry.
type Crop struct {
	Code              string           `json:"code"`
	NameKey           string           `json:"name_key"`
	Seasons           []string         `json:"seasons"`
	WaterNeedMM       int              `json:"water_need_mm"`
	Textures          []string         `json:"textures"`
	PHMin             float64          `json:"ph_min"`
	PHMax             float64          `json:"ph_max"`
	NitrogenDeltaKgHa int              `json:"nitrogen_delta_kg_ha"`
	DurationDays      int              `json:"duration_days"`
	YieldKgHa         Yield            `json:"yield_kg_ha"`
	PricePoishaPerKg  int64            `json:"price_poisha_per_kg"`
	CostPoishaPerHa   map[string]int64 `json:"cost_poisha_per_ha"`
}

// CropList lists crops.
type CropList struct {
	Crops []Crop `json:"crops"`
}

// Handlers groups the farm endpoints.
type Handlers struct {
	Create  usecase.CreateField
	Get     usecase.GetField
	List    usecase.ListFields
	Update  usecase.UpdateField
	Delete  usecase.DeleteField
	PutSoil usecase.PutSoil
	AddPlot usecase.AddPlot
	V       *validator.Validator
}

// Routes declares the endpoints.
func (h Handlers) Routes() []httpx.Route {
	return []httpx.Route{
		{Method: http.MethodGet, Path: "/v1/crops", Tag: "farm", Summary: "Crop catalogue", Response: CropList{}, Handler: h.crops},
		{Method: http.MethodGet, Path: "/v1/crops/{code}", Tag: "farm", Summary: "One crop", Response: Crop{}, Handler: h.crop},
		{Method: http.MethodGet, Path: "/v1/fields", Permission: string(authz.FieldRead), Tag: "farm", Summary: "List fields", Query: OwnerQuery{}, Response: FieldList{}, Handler: h.list},
		{Method: http.MethodPost, Path: "/v1/fields", Permission: string(authz.FieldCreate), Tag: "farm", Summary: "Create a field", Request: FieldRequest{}, Response: Field{}, Status: http.StatusCreated, Handler: h.create},
		{Method: http.MethodGet, Path: "/v1/fields/{id}", Permission: string(authz.FieldRead), Tag: "farm", Summary: "Get a field", Response: Field{}, Handler: h.get},
		{Method: http.MethodPatch, Path: "/v1/fields/{id}", Permission: string(authz.FieldWrite), Tag: "farm", Summary: "Update a field", Request: FieldPatchRequest{}, Response: Field{}, Handler: h.update},
		{Method: http.MethodDelete, Path: "/v1/fields/{id}", Permission: string(authz.FieldWrite), Tag: "farm", Summary: "Delete a field", Status: http.StatusNoContent, Handler: h.delete},
		{Method: http.MethodPut, Path: "/v1/fields/{id}/soil", Permission: string(authz.FieldWrite), Tag: "farm", Summary: "Record a soil test", Request: Soil{}, Response: Field{}, Handler: h.soil},
		{Method: http.MethodPost, Path: "/v1/fields/{id}/plots", Permission: string(authz.FieldWrite), Tag: "farm", Summary: "Add a plot", Request: PlotRequest{}, Response: Plot{}, Status: http.StatusCreated, Handler: h.plot},
	}
}

// ToArea renders an area in every unit.
func ToArea(a domain.Area, unit domain.Unit) Area {
	return Area{Unit: string(unit), ValueMilli: a.Milli(unit), DecimalMilli: a.Milli(domain.Decimal), BighaMilli: a.Milli(domain.Bigha),
		AcreMilli: a.Milli(domain.Acre), HectareMilli: a.Milli(domain.Hectare)}
}

func toPlot(p domain.Plot, unit domain.Unit) Plot {
	return Plot{ID: p.ID, Name: p.Name, Area: ToArea(p.Area, unit), CropCode: p.CropCode, SownOn: p.SownOn}
}

func toField(f domain.Field) Field {
	out := Field{ID: f.ID, OwnerID: f.OwnerID, Name: f.Name, Area: ToArea(f.Area, f.AreaUnit),
		Location: Location{Lat: f.Location.Lat(), Lng: f.Location.Lng()}, District: f.District, Irrigation: f.Irrigation,
		Plots: make([]Plot, len(f.Plots)), CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt}
	for i, p := range f.Plots {
		out.Plots[i] = toPlot(p, f.AreaUnit)
	}
	if s := f.Soil; s != nil {
		out.Soil = &Soil{Texture: string(s.Texture), PH: float64(s.PHx10) / 10, NitrogenKgHa: s.NitrogenKgHa, PhosphorusKgHa: s.PhosphorusKgHa,
			PotassiumKgHa: s.PotassiumKgHa, OrganicMatterPct: float64(s.OrganicMatterX10) / 10, TestedOn: s.TestedOn}
	}
	return out
}

// ToCrop renders a catalogue entry.
func ToCrop(c domain.Crop) Crop {
	ts := make([]string, len(c.Textures))
	for i, t := range c.Textures {
		ts[i] = string(t)
	}
	return Crop{Code: c.Code, NameKey: "crop." + c.Code + ".name", Seasons: c.Seasons, WaterNeedMM: c.WaterNeedMM, Textures: ts,
		PHMin: float64(c.PHMinX10) / 10, PHMax: float64(c.PHMaxX10) / 10, NitrogenDeltaKgHa: c.NitrogenDeltaKgHa,
		DurationDays: c.DurationDays, YieldKgHa: Yield{c.YieldKgHa.Low, c.YieldKgHa.Likely, c.YieldKgHa.High},
		PricePoishaPerKg: c.PricePoishaPerKg, CostPoishaPerHa: c.CostPoishaPerHa}
}

func (h Handlers) crops(w http.ResponseWriter, _ *http.Request) error {
	cs := domain.Catalogue()
	out := CropList{Crops: make([]Crop, len(cs))}
	for i, c := range cs {
		out.Crops[i] = ToCrop(c)
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h Handlers) crop(w http.ResponseWriter, r *http.Request) error {
	c, err := domain.FindCrop(r.PathValue("code"))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, ToCrop(c))
}

func (h Handlers) list(w http.ResponseWriter, r *http.Request) error {
	fs, err := h.List.Execute(r.Context(), r.URL.Query().Get("owner_id"))
	if err != nil {
		return err
	}
	out := FieldList{Fields: make([]Field, len(fs))}
	for i, f := range fs {
		out.Fields[i] = toField(f)
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func soilInput(s Soil) usecase.SoilInput {
	return usecase.SoilInput{Texture: s.Texture, PH: s.PH, NitrogenKgHa: s.NitrogenKgHa, PhosphorusKgHa: s.PhosphorusKgHa,
		PotassiumKgHa: s.PotassiumKgHa, OrganicMatterPct: s.OrganicMatterPct, TestedOn: s.TestedOn}
}

func (h Handlers) create(w http.ResponseWriter, r *http.Request) error {
	var in FieldRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	fi := usecase.FieldInput{Name: in.Name, Area: usecase.AreaInput{ValueMilli: in.Area.ValueMilli, Unit: in.Area.Unit},
		Lat: in.Location.Lat, Lng: in.Location.Lng, District: in.District, Irrigation: in.Irrigation}
	if in.Soil != nil {
		s := soilInput(*in.Soil)
		fi.Soil = &s
	}
	f, err := h.Create.Execute(r.Context(), fi)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, toField(f))
}

func (h Handlers) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	f, err := h.Get.Execute(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, toField(f))
}

func (h Handlers) update(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in FieldPatchRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	p := usecase.FieldPatch{Name: in.Name, District: in.District, Irrigation: in.Irrigation}
	if in.Area != nil {
		p.Area = &usecase.AreaInput{ValueMilli: in.Area.ValueMilli, Unit: in.Area.Unit}
	}
	if in.Location != nil {
		p.Lat, p.Lng = &in.Location.Lat, &in.Location.Lng
	}
	f, err := h.Update.Execute(r.Context(), id, p)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, toField(f))
}

func (h Handlers) delete(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	if err := h.Delete.Execute(r.Context(), id); err != nil {
		return err
	}
	return httpx.NoContent(w)
}

func (h Handlers) soil(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in Soil
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	f, err := h.PutSoil.Execute(r.Context(), id, soilInput(in))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, toField(f))
}

func (h Handlers) plot(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in PlotRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	p, err := h.AddPlot.Execute(r.Context(), id, usecase.PlotInput{Name: in.Name, Area: usecase.AreaInput{ValueMilli: in.Area.ValueMilli, Unit: in.Area.Unit},
		CropCode: in.CropCode, SownOn: in.SownOn})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, toPlot(p, domain.Unit(in.Area.Unit)))
}
