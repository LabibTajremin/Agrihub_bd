package advisory_test

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"testing"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter/stub"
	farmdomain "github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/test/apitest"
)

var update = flag.Bool("update", false, "rewrite golden files")

const (
	fieldID = "00000000-0000-7000-8000-00000000f001"
	bareID  = "00000000-0000-7000-8000-00000000f002"
	owner   = "00000000-0000-7000-8000-0000000000a1"
	other   = "00000000-0000-7000-8000-0000000000b2"
	missing = "00000000-0000-7000-8000-0000000000ff"
)

var errFieldNotFound = errs.NotFound("farm.field_not_found")

type fields map[string]domain.Field

func (f fields) Field(_ context.Context, id string) (domain.Field, error) {
	if x, ok := f[id]; ok {
		return x, nil
	}
	return domain.Field{}, errFieldNotFound
}

type catalogue []domain.Crop

func (c catalogue) Crops() []domain.Crop { return c }

// farmCatalogue converts the real farm catalogue so the golden test uses production data.
func farmCatalogue() catalogue {
	var out catalogue
	for _, c := range farmdomain.Catalogue() {
		ts := make([]string, len(c.Textures))
		for i, t := range c.Textures {
			ts[i] = string(t)
		}
		out = append(out, domain.Crop{Code: c.Code, Seasons: c.Seasons, WaterNeedMM: c.WaterNeedMM, Textures: ts, PHMinX10: c.PHMinX10,
			PHMaxX10: c.PHMaxX10, NitrogenDeltaKgHa: c.NitrogenDeltaKgHa, YieldKgHa: domain.Yield(c.YieldKgHa), PricePoishaPerKg: c.PricePoishaPerKg,
			SeedAvailBP: c.SeedAvailBP, MarketDemandBP: c.MarketDemandBP, PestRiskBP: c.PestRiskBP, CostPoishaPerHa: c.CostPoishaPerHa})
	}
	return out
}

type weather struct{ err error }

func (w weather) Outlook(context.Context, float64, float64) (domain.Outlook, error) {
	if w.err != nil {
		return domain.Outlook{}, w.err
	}
	return domain.Outlook{RainMM: map[string]int{"aman": 1500, "boro": 120, "aus": 700}, AgeSeconds: 3600}, nil
}

var weights = domain.Weights{Soil: 0.30, Water: 0.25, Pest: 0.15, Market: 0.20, Seed: 0.10}

func fixture() fields {
	return fields{
		fieldID: {ID: fieldID, OwnerID: owner, Lat: 24.85, Lng: 89.37, AreaNano: 10_000_000_000_000, Texture: "loam", PHx10: 65,
			NitrogenKgHa: 120, Irrigation: "partial", CurrentCrop: "rice_aman"},
		bareID: {ID: bareID, OwnerID: owner, AreaNano: 5_000_000_000_000},
	}
}

func setup(t *testing.T, w domain.WeatherSource, crops domain.CropSource) *apitest.API {
	t.Helper()
	a := apitest.New(t, nil)
	m, err := advisory.New(advisory.Deps{Kernel: a.Kernel, Fields: fixture(), Crops: crops, Weather: w, Narrator: stub.Engine{}, Weights: weights, TopN: 5})
	if err != nil {
		t.Fatal(err)
	}
	a.Mount(m.Routes())
	return a
}

func golden(t *testing.T, name string, v any) {
	t.Helper()
	got, _ := json.MarshalIndent(v, "", "  ")
	path := "testdata/" + name + ".golden.json"
	if *update {
		if err := os.WriteFile(path, append(got, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil || string(want) != string(got)+"\n" {
		t.Fatalf("%s drifted (run with -update):\n%s", name, got)
	}
}

// TestRecommendations_GoldenFixedField pins the ranking for a fixed field.
func TestRecommendations_GoldenFixedField(t *testing.T) {
	a := setup(t, weather{}, farmCatalogue())
	tok := a.Token(owner, authz.Farmer)
	var boro transport.Recommendations
	a.Do("GET", "/v1/advisory/fields/"+fieldID+"/recommendations?season=boro", nil, tok).Expect(t, 200).Decode(t, &boro)
	golden(t, "recommendations_boro", boro)
	var rot transport.Rotation
	a.Do("GET", "/v1/advisory/fields/"+fieldID+"/rotation?start=boro&seasons=3", nil, tok).Expect(t, 200).Decode(t, &rot)
	golden(t, "rotation_boro_3", rot)
	for i := 1; i < len(boro.Items); i++ {
		if boro.Items[i].Score > boro.Items[i-1].Score {
			t.Fatal("ranked best first")
		}
	}
}

func TestRecommendations_DefaultsAndDegradation(t *testing.T) {
	a := setup(t, weather{err: errors.New("provider down")}, farmCatalogue())
	var r transport.Recommendations
	a.Do("GET", "/v1/advisory/fields/"+fieldID+"/recommendations", nil, a.Token(owner, authz.Farmer)).Expect(t, 200).Decode(t, &r)
	if r.Season != "aus" || !r.Outlook.Stale || r.Outlook.RainMM != domain.DefaultRainMM["aus"] {
		t.Fatalf("season from clock (March → aus); climatology when weather is down: %+v", r)
	}
	if c := a.Do("GET", "/v1/advisory/fields/"+fieldID+"/recommendations?season=winter", nil, a.Token(owner, authz.Farmer)).Expect(t, 400).ErrorCode(); c != "advisory.invalid_input" {
		t.Fatal(c)
	}
	empty := setup(t, weather{}, catalogue{})
	if c := empty.Do("GET", "/v1/advisory/fields/"+fieldID+"/recommendations?season=aman", nil, empty.Token(owner, authz.Farmer)).Expect(t, 404).ErrorCode(); c != "advisory.no_candidates" {
		t.Fatal(c)
	}
}

func TestAccessPolicy(t *testing.T) {
	a := setup(t, weather{}, farmCatalogue())
	paths := []string{"/recommendations", "/crops/wheat", "/rotation"}
	for _, p := range paths {
		url := "/v1/advisory/fields/" + fieldID + p
		a.Do("GET", url, nil, a.Token(owner, authz.Farmer)).Expect(t, 200)
		a.Do("GET", url, nil, a.Token(other, authz.Farmer)).Expect(t, 403)
		a.Do("GET", url, nil, a.Token(other, authz.FieldOfficer)).Expect(t, 200)
		a.Do("GET", url, nil, "").Expect(t, 401)
		a.Do("GET", "/v1/advisory/fields/"+missing+p, nil, a.Token(owner, authz.Farmer)).Expect(t, 404)
		a.Do("GET", "/v1/advisory/fields/nope"+p, nil, a.Token(owner, authz.Farmer)).Expect(t, 400)
	}
	a.Do("POST", "/v1/advisory/fields/"+fieldID+"/crops/wheat/roi", transport.ROIRequest{}, a.Token(other, authz.Farmer)).Expect(t, 403)
	a.Do("POST", "/v1/advisory/fields/nope/crops/wheat/roi", transport.ROIRequest{}, a.Token(owner, authz.Farmer)).Expect(t, 400)
}

func TestCropDetailAndROI(t *testing.T) {
	a := setup(t, weather{}, farmCatalogue())
	tok := a.Token(owner, authz.Farmer)
	var d transport.CropDetail
	a.Do("GET", "/v1/advisory/fields/"+fieldID+"/crops/rice_boro", nil, tok).Expect(t, 200).Decode(t, &d)
	if d.Season != "boro" || d.YieldKg.Likely != 6000 || d.ROI.GrossPoisha.Likely != 6000*3000 ||
		d.ROI.NetPoisha.Likely != d.ROI.GrossPoisha.Likely-d.ROI.TotalCost || d.NameKey != "crop.rice_boro.name" {
		t.Fatalf("%+v", d)
	}
	a.Do("GET", "/v1/advisory/fields/"+fieldID+"/crops/kale", nil, tok).Expect(t, 400)
	a.Do("GET", "/v1/advisory/fields/"+fieldID+"/crops/wheat?season=monsoon", nil, tok).Expect(t, 400)
	var roi transport.ROI
	a.Do("POST", "/v1/advisory/fields/"+bareID+"/crops/wheat/roi", transport.ROIRequest{CostsPoisha: map[string]int64{"seed": 100000, "labour": 400000}}, tok).
		Expect(t, 200).Decode(t, &roi)
	if roi.TotalCost != 500000 || roi.YieldKg.Likely != 1750 || roi.NetPoisha.Likely != 1750*3500-500000 {
		t.Fatalf("half a hectare of wheat: %+v", roi)
	}
	var def transport.ROI
	a.Do("POST", "/v1/advisory/fields/"+bareID+"/crops/wheat/roi", transport.ROIRequest{}, tok).Expect(t, 200).Decode(t, &def)
	if def.CostsPoisha["seed"] != 300000 {
		t.Fatal("catalogue costs scaled to the area", def.CostsPoisha)
	}
	a.Do("POST", "/v1/advisory/fields/"+bareID+"/crops/wheat/roi", transport.ROIRequest{CostsPoisha: map[string]int64{"x": -1}}, tok).Expect(t, 400)
	a.Do("POST", "/v1/advisory/fields/"+bareID+"/crops/kale/roi", transport.ROIRequest{}, tok).Expect(t, 400)
	a.Do("POST", "/v1/advisory/fields/"+missing+"/crops/wheat/roi", transport.ROIRequest{}, tok).Expect(t, 404)
	a.Do("POST", "/v1/advisory/fields/"+bareID+"/crops/wheat/roi", "{", tok).Expect(t, 400)
}

func TestRotation_Parameters(t *testing.T) {
	a := setup(t, weather{}, farmCatalogue())
	tok := a.Token(owner, authz.Farmer)
	var r transport.Rotation
	a.Do("GET", "/v1/advisory/fields/"+bareID+"/rotation?start=aman", nil, tok).Expect(t, 200).Decode(t, &r)
	if len(r.Steps) != 3 || r.Steps[0].NitrogenBefore != usecase.DefaultNitrogenKgHa || r.Steps[0].Season != "aman" {
		t.Fatalf("%+v", r)
	}
	a.Do("GET", "/v1/advisory/fields/"+bareID+"/rotation?seasons=9", nil, tok).Expect(t, 400)
	a.Do("GET", "/v1/advisory/fields/"+bareID+"/rotation?start=summer", nil, tok).Expect(t, 400)
}

func TestNarrate(t *testing.T) {
	a := setup(t, weather{}, farmCatalogue())
	tok := a.Token(owner, authz.Guest)
	var n transport.Narration
	a.Do("POST", "/v1/advisory/narrate", transport.NarrateRequest{Chart: "roi", Lang: "bn", Data: map[string]float64{"net": 1}}, tok).Expect(t, 200).Decode(t, &n)
	if n.TextKey != "narration.chart.roi" || n.VoiceKey != n.TextKey {
		t.Fatal(n)
	}
	if c := a.Do("POST", "/v1/advisory/narrate", transport.NarrateRequest{Chart: "pie", Lang: "bn"}, tok).Expect(t, 400).ErrorCode(); c != "ai.invalid_input" {
		t.Fatal(c)
	}
	a.Do("POST", "/v1/advisory/narrate", transport.NarrateRequest{Chart: "roi"}, tok).Expect(t, 400)
}

func TestUseCases_Guards(t *testing.T) {
	a := apitest.New(t, nil)
	d := usecase.Deps{Fields: fixture(), Crops: farmCatalogue(), Weather: weather{}, Narrator: stub.Engine{}, Weights: weights, TopN: 5, Clock: a.Clock}
	anon := context.Background()
	if _, err := (usecase.Narrate{Deps: d}).Execute(anon, aiadapter.NarrateInput{}); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	if _, err := (usecase.Recommend{Deps: d}).Execute(anon, fieldID, ""); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	farmer := authn.WithPrincipal(anon, authn.Principal{Subject: owner, Role: authz.Farmer})
	for _, n := range []int{0, 7} {
		if _, err := (usecase.PlanRotation{Deps: d}).Execute(farmer, fieldID, "aman", n); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%d: %v", n, err)
		}
	}
	if _, err := advisory.New(advisory.Deps{Weights: domain.Weights{Soil: 1, Water: 1}}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatal("weights must sum to 1.0 at boot", err)
	}
	if len(advisory.Errors()) != 3 {
		t.Fatal()
	}
}
