// Package transport exposes the Crop Advisor over HTTP.
package transport

import (
	"net/http"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// SeasonQuery selects a season.
type SeasonQuery struct {
	Season string `query:"season"`
}

// RotationQuery configures a rotation plan.
type RotationQuery struct {
	Start   string `query:"start"`
	Seasons int    `query:"seasons"`
}

// Criteria are normalised criterion values in [0,1].
type Criteria struct {
	Soil   float64 `json:"soil"`
	Water  float64 `json:"water"`
	Pest   float64 `json:"pest"`
	Market float64 `json:"market"`
	Seed   float64 `json:"seed"`
}

// Triple is a low / likely / high value.
type Triple struct {
	Low    int64 `json:"low"`
	Likely int64 `json:"likely"`
	High   int64 `json:"high"`
}

// Outlook is the seasonal forecast used.
type Outlook struct {
	RainMM         int   `json:"rain_mm"`
	DataAgeSeconds int64 `json:"data_age_seconds"`
	Stale          bool  `json:"stale"`
}

// Recommendation is one ranked crop.
type Recommendation struct {
	CropCode string   `json:"crop_code"`
	NameKey  string   `json:"name_key"`
	Score    float64  `json:"score"`
	Criteria Criteria `json:"criteria"`
	YieldKg  Triple   `json:"yield_kg"`
}

// Recommendations is the ranked list.
type Recommendations struct {
	Season  string           `json:"season"`
	Outlook Outlook          `json:"outlook"`
	Items   []Recommendation `json:"items"`
}

// ROI is a priced estimate in poisha.
type ROI struct {
	YieldKg     Triple           `json:"yield_kg"`
	CostsPoisha map[string]int64 `json:"costs_poisha"`
	GrossPoisha Triple           `json:"gross_poisha"`
	TotalCost   int64            `json:"total_cost_poisha"`
	NetPoisha   Triple           `json:"net_poisha"`
	ReturnBP    int64            `json:"return_bp"`
}

// CropDetail evaluates one crop on a field.
type CropDetail struct {
	Recommendation
	Season  string  `json:"season"`
	Outlook Outlook `json:"outlook"`
	ROI     ROI     `json:"roi"`
}

// ROIRequest overrides costs (poisha per category); omit for catalogue defaults.
type ROIRequest struct {
	CostsPoisha map[string]int64 `json:"costs_poisha"`
}

// RotationStep is one season of a plan.
type RotationStep struct {
	Season         string  `json:"season"`
	CropCode       string  `json:"crop_code"`
	NameKey        string  `json:"name_key"`
	NitrogenBefore int     `json:"nitrogen_before_kg_ha"`
	NitrogenAfter  int     `json:"nitrogen_after_kg_ha"`
	Score          float64 `json:"score"`
}

// Rotation is a rotation plan.
type Rotation struct {
	Steps   []RotationStep `json:"steps"`
	Deficit bool           `json:"nitrogen_deficit"`
}

// NarrateRequest asks for a chart narration.
type NarrateRequest struct {
	Chart string             `json:"chart" validate:"required,max=20"`
	Lang  string             `json:"lang" validate:"required,max=5"`
	Data  map[string]float64 `json:"data"`
}

// Narration is a dictionary key for text and pre-recorded voice.
type Narration struct {
	TextKey  string            `json:"text_key"`
	VoiceKey string            `json:"voice_key"`
	Params   map[string]string `json:"params"`
	Text     string            `json:"text,omitempty"`
}

// Handlers groups the advisory endpoints.
type Handlers struct {
	Recommend usecase.Recommend
	Detail    usecase.CropDetail
	ROI       usecase.EstimateROI
	Rotation  usecase.PlanRotation
	Narrate   usecase.Narrate
	V         *validator.Validator
}

// Routes declares the endpoints.
func (h Handlers) Routes() []httpx.Route {
	perm := string(authz.AdvisoryRead)
	return []httpx.Route{
		{Method: http.MethodGet, Path: "/v1/advisory/fields/{id}/recommendations", Permission: perm, Tag: "advisory", Summary: "Ranked crops for a field",
			Query: SeasonQuery{}, Response: Recommendations{}, Handler: h.recommend},
		{Method: http.MethodGet, Path: "/v1/advisory/fields/{id}/crops/{code}", Permission: perm, Tag: "advisory", Summary: "One crop on a field",
			Query: SeasonQuery{}, Response: CropDetail{}, Handler: h.detail},
		{Method: http.MethodPost, Path: "/v1/advisory/fields/{id}/crops/{code}/roi", Permission: perm, Tag: "advisory", Summary: "ROI with custom costs",
			Request: ROIRequest{}, Response: ROI{}, Handler: h.roi},
		{Method: http.MethodGet, Path: "/v1/advisory/fields/{id}/rotation", Permission: perm, Tag: "advisory", Summary: "Nitrogen-balanced rotation plan",
			Query: RotationQuery{}, Response: Rotation{}, Handler: h.rotation},
		{Method: http.MethodPost, Path: "/v1/advisory/narrate", Permission: perm, Tag: "advisory", Summary: "Narrate a chart (text key + voice key)",
			Request: NarrateRequest{}, Response: Narration{}, Handler: h.narrate},
	}
}

func triple(y domain.Yield) Triple { return Triple{y.Low, y.Likely, y.High} }

func toOutlook(o domain.Outlook, season string) Outlook {
	return Outlook{RainMM: o.RainMM[season], DataAgeSeconds: o.AgeSeconds, Stale: o.Stale}
}

func toRec(s domain.Scored, y domain.Yield) Recommendation {
	c := s.Criteria
	return Recommendation{CropCode: s.Crop.Code, NameKey: "crop." + s.Crop.Code + ".name", Score: s.Score,
		Criteria: Criteria{c.Soil, c.Water, c.Pest, c.Market, c.Seed}, YieldKg: triple(y)}
}

func toROI(y domain.Yield, costs map[string]int64, r domain.ROI) ROI {
	return ROI{YieldKg: triple(y), CostsPoisha: costs, GrossPoisha: triple(r.Gross), TotalCost: r.TotalCost, NetPoisha: triple(r.Net), ReturnBP: r.ReturnBP}
}

func (h Handlers) recommend(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	rec, err := h.Recommend.Execute(r.Context(), id, r.URL.Query().Get("season"))
	if err != nil {
		return err
	}
	out := Recommendations{Season: rec.Season, Outlook: toOutlook(rec.Outlook, rec.Season), Items: make([]Recommendation, len(rec.Items))}
	for i, it := range rec.Items {
		out.Items[i] = toRec(it.Scored, it.Yield)
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h Handlers) detail(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	d, err := h.Detail.Execute(r.Context(), id, r.PathValue("code"), r.URL.Query().Get("season"))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, CropDetail{Recommendation: toRec(d.Scored, d.Yield), Season: d.Season,
		Outlook: toOutlook(d.Outlook, d.Season), ROI: toROI(d.Yield, d.Costs, d.ROI)})
}

func (h Handlers) roi(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in ROIRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	res, err := h.ROI.Execute(r.Context(), id, r.PathValue("code"), in.CostsPoisha)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, toROI(res.Yield, res.Costs, res.ROI))
}

func (h Handlers) rotation(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	n, err := httpx.QueryInt(r, "seasons", 3, 1, usecase.MaxRotationSeasons)
	if err != nil {
		return err
	}
	plan, err := h.Rotation.Execute(r.Context(), id, r.URL.Query().Get("start"), int(n))
	if err != nil {
		return err
	}
	out := Rotation{Deficit: plan.Deficit, Steps: make([]RotationStep, len(plan.Steps))}
	for i, s := range plan.Steps {
		out.Steps[i] = RotationStep{Season: s.Season, CropCode: s.Crop, NameKey: "crop." + s.Crop + ".name",
			NitrogenBefore: s.NitrogenBefore, NitrogenAfter: s.NitrogenAfter, Score: s.Score}
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h Handlers) narrate(w http.ResponseWriter, r *http.Request) error {
	var in NarrateRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	res, err := h.Narrate.Execute(r.Context(), aiadapter.NarrateInput{ChartKind: in.Chart, Lang: in.Lang, Data: in.Data})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, Narration{TextKey: res.TextKey, VoiceKey: res.VoiceKey, Params: res.Params, Text: res.Text})
}
