//go:build integration

package farm_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/test/apitest"
	"github.com/labibtajremin/agrihub_bd/backend/test/harness"
)

var (
	ctx  = context.Background()
	boom = errors.New("boom")
)

const (
	owner  = "00000000-0000-7000-8000-0000000000a1"
	other  = "00000000-0000-7000-8000-0000000000b2"
	nobody = "00000000-0000-7000-8000-0000000000ff"
)

func setup(t *testing.T, db *database.DB) (*apitest.API, *farm.Module) {
	t.Helper()
	a := apitest.New(t, db)
	m, err := farm.New(farm.Deps{Kernel: a.Kernel})
	if err != nil {
		t.Fatal(err)
	}
	a.Mount(m.Routes())
	return a, m
}

func fieldReq() transport.FieldRequest {
	return transport.FieldRequest{Name: "North paddy", Area: transport.AreaIn{ValueMilli: 2500, Unit: "bigha"},
		Location: transport.Location{Lat: 24.8465, Lng: 89.3773}, District: "Bogura", Irrigation: "partial",
		Soil: &transport.Soil{Texture: "clay_loam", PH: 6.2, NitrogenKgHa: 120, PhosphorusKgHa: 20, PotassiumKgHa: 80, OrganicMatterPct: 1.5}}
}

func create(t *testing.T, a *apitest.API, token string) transport.Field {
	t.Helper()
	var f transport.Field
	a.Do("POST", "/v1/fields", fieldReq(), token).Expect(t, 201).Decode(t, &f)
	return f
}

func TestCrops_PublicCatalogue(t *testing.T) {
	a, _ := setup(t, harness.DB(t))
	var list transport.CropList
	a.Do("GET", "/v1/crops", nil, "").Expect(t, 200).Decode(t, &list)
	if len(list.Crops) != 12 || list.Crops[0].NameKey != "crop."+list.Crops[0].Code+".name" {
		t.Fatalf("%+v", list.Crops[0])
	}
	var c transport.Crop
	a.Do("GET", "/v1/crops/lentil", nil, "").Expect(t, 200).Decode(t, &c)
	if c.NitrogenDeltaKgHa <= 0 || c.PHMin != 6 {
		t.Fatal(c)
	}
	a.Do("GET", "/v1/crops/kale", nil, "").Expect(t, 404)
}

func TestField_LifecycleAndUnits(t *testing.T) {
	a, _ := setup(t, harness.DB(t))
	tok := a.Token(owner, authz.Farmer)
	f := create(t, a, tok)
	if f.OwnerID != owner || f.Area.DecimalMilli != 82500 || f.Area.BighaMilli != 2500 || f.Area.Unit != "bigha" ||
		f.Soil == nil || f.Soil.PH != 6.2 || f.Location.Lat != 24.8465 || len(f.Plots) != 0 {
		t.Fatalf("%+v", f)
	}
	var plot transport.Plot
	a.Do("POST", "/v1/fields/"+f.ID+"/plots", transport.PlotRequest{Name: "A", Area: transport.AreaIn{ValueMilli: 1000, Unit: "bigha"}, CropCode: "rice_aman"}, tok).
		Expect(t, 201).Decode(t, &plot)
	if plot.Area.DecimalMilli != 33000 {
		t.Fatal(plot)
	}
	if c := a.Do("POST", "/v1/fields/"+f.ID+"/plots", transport.PlotRequest{Name: "B", Area: transport.AreaIn{ValueMilli: 2000, Unit: "bigha"}, CropCode: "wheat"}, tok).
		Expect(t, 400).ErrorCode(); c != "farm.invalid_area" {
		t.Fatal("plots may not exceed the field", c)
	}
	a.Do("POST", "/v1/fields/"+f.ID+"/plots", transport.PlotRequest{Name: "C", Area: transport.AreaIn{ValueMilli: 1, Unit: "decimal"}, CropCode: "kale"}, tok).Expect(t, 404)
	name, irrigation := "South paddy", "full"
	var upd transport.Field
	a.Do("PATCH", "/v1/fields/"+f.ID, transport.FieldPatchRequest{Name: &name, Irrigation: &irrigation,
		Area: &transport.AreaIn{ValueMilli: 1500, Unit: "hectare"}, Location: &transport.Location{Lat: 23.1, Lng: 90.2}}, tok).Expect(t, 200).Decode(t, &upd)
	if upd.Name != name || upd.Irrigation != "full" || upd.Area.HectareMilli != 1500 || upd.Location.Lng != 90.2 || len(upd.Plots) != 1 {
		t.Fatalf("%+v", upd)
	}
	var soiled transport.Field
	a.Do("PUT", "/v1/fields/"+f.ID+"/soil", transport.Soil{Texture: "loam", PH: 7.1}, tok).Expect(t, 200).Decode(t, &soiled)
	if soiled.Soil.Texture != "loam" || soiled.Soil.PH != 7.1 {
		t.Fatal(soiled.Soil)
	}
	var list transport.FieldList
	a.Do("GET", "/v1/fields", nil, tok).Expect(t, 200).Decode(t, &list)
	if len(list.Fields) != 1 || list.Fields[0].Soil.Texture != "loam" {
		t.Fatal(list)
	}
	a.Do("DELETE", "/v1/fields/"+f.ID, nil, tok).Expect(t, 204)
	a.Do("GET", "/v1/fields/"+f.ID, nil, tok).Expect(t, 404)
	flushed, _ := a.Relay.Flush(ctx)
	if flushed.Dispatched != 4 {
		t.Fatal("created, updated, soil, deleted events", flushed)
	}
}

// TestField_OwnershipPolicyForAllRoles checks every role × action × ownership.
func TestField_OwnershipPolicyForAllRoles(t *testing.T) {
	a, _ := setup(t, harness.DB(t))
	f := create(t, a, a.Token(owner, authz.Farmer))
	type want struct{ readOwn, readOther, writeOwn, writeOther, create, listOther int }
	cases := map[authz.Role]want{
		authz.Guest:        {403, 403, 403, 403, 403, 403},
		authz.Farmer:       {200, 403, 200, 403, 201, 403},
		authz.FieldOfficer: {200, 200, 200, 403, 201, 200},
		authz.Agronomist:   {200, 200, 200, 403, 201, 200},
		authz.Admin:        {200, 200, 200, 200, 201, 200},
	}
	name := "renamed"
	for role, w := range cases {
		mine := a.Token(owner, role)
		theirs := a.Token(other, role)
		check := func(action string, got, exp int) {
			if got != exp {
				t.Errorf("%s %s: want %d got %d", role, action, exp, got)
			}
		}
		check("read own", a.Do("GET", "/v1/fields/"+f.ID, nil, mine).Code, w.readOwn)
		check("read other", a.Do("GET", "/v1/fields/"+f.ID, nil, theirs).Code, w.readOther)
		check("write own", a.Do("PATCH", "/v1/fields/"+f.ID, transport.FieldPatchRequest{Name: &name}, mine).Code, w.writeOwn)
		check("write other", a.Do("PATCH", "/v1/fields/"+f.ID, transport.FieldPatchRequest{Name: &name}, theirs).Code, w.writeOther)
		check("create", a.Do("POST", "/v1/fields", fieldReq(), theirs).Code, w.create)
		check("list other", a.Do("GET", "/v1/fields?owner_id="+owner, nil, theirs).Code, w.listOther)
	}
	a.Do("GET", "/v1/fields/"+nobody, nil, a.Token(owner, authz.Farmer)).Expect(t, 404)
	a.Do("GET", "/v1/fields/not-a-uuid", nil, a.Token(owner, authz.Farmer)).Expect(t, 400)
	a.Do("GET", "/v1/fields", nil, "").Expect(t, 401)
}

func TestField_ValidationOverHTTP(t *testing.T) {
	a, _ := setup(t, harness.DB(t))
	tok := a.Token(owner, authz.Farmer)
	bad := fieldReq()
	bad.Area.Unit = "rood"
	a.Do("POST", "/v1/fields", bad, tok).Expect(t, 400)
	huge := fieldReq()
	huge.Area = transport.AreaIn{ValueMilli: 1 << 60, Unit: "hectare"}
	if c := a.Do("POST", "/v1/fields", huge, tok).Expect(t, 400).ErrorCode(); c != "farm.invalid_area" {
		t.Fatal(c)
	}
	for _, path := range []string{"/v1/fields/x", "/v1/fields/x/soil", "/v1/fields/x/plots"} {
		method := map[string]string{"/v1/fields/x": "PATCH", "/v1/fields/x/soil": "PUT", "/v1/fields/x/plots": "POST"}[path]
		a.Do(method, path, "{}", tok).Expect(t, 400)
		a.Do(method, "/v1/fields/"+nobody+path[len("/v1/fields/x"):], "{", tok).Expect(t, 400)
	}
	a.Do("DELETE", "/v1/fields/x", nil, tok).Expect(t, 400)
	a.Do("POST", "/v1/fields", "{", tok).Expect(t, 400)
	a.Do("DELETE", "/v1/fields/"+nobody, nil, tok).Expect(t, 404)
	a.Do("PUT", "/v1/fields/"+nobody+"/soil", transport.Soil{Texture: "loam", PH: 7}, tok).Expect(t, 404)
	a.Do("POST", "/v1/fields/"+nobody+"/plots", transport.PlotRequest{Name: "A", Area: transport.AreaIn{ValueMilli: 1, Unit: "decimal"}, CropCode: "wheat"}, tok).Expect(t, 404)
	a.Do("PATCH", "/v1/fields/"+nobody, transport.FieldPatchRequest{}, tok).Expect(t, 404)
}

// ---- use-case level ----

func deps(t *testing.T, db *database.DB) usecase.Deps {
	a := apitest.New(t, db)
	return usecase.Deps{Fields: repository.Fields{DB: db}, Tx: db, Clock: a.Clock, IDs: a.Kernel.IDs, Events: a.Kernel.Events, Factory: a.Kernel.Factory()}
}

var farmer = authn.WithPrincipal(ctx, authn.Principal{Subject: owner, Role: authz.Farmer})

func input() usecase.FieldInput {
	return usecase.FieldInput{Name: "F", Area: usecase.AreaInput{ValueMilli: 1000, Unit: "decimal"}, Lat: 23, Lng: 90, Irrigation: "none",
		Soil: &usecase.SoilInput{Texture: "loam", PH: 6.5}}
}

func TestUseCases_Validation(t *testing.T) {
	d := deps(t, harness.DB(t))
	cases := map[string]func(*usecase.FieldInput){
		"area":       func(in *usecase.FieldInput) { in.Area.ValueMilli = 0 },
		"location":   func(in *usecase.FieldInput) { in.Lat = 100 },
		"name":       func(in *usecase.FieldInput) { in.Name = "" },
		"irrigation": func(in *usecase.FieldInput) { in.Irrigation = "flood" },
		"soil":       func(in *usecase.FieldInput) { in.Soil.PH = 1 },
	}
	for name, mutate := range cases {
		in := input()
		mutate(&in)
		if _, err := (usecase.CreateField{Deps: d}).Execute(farmer, in); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	f, err := usecase.CreateField{Deps: d}.Execute(farmer, input())
	if err != nil {
		t.Fatal(err)
	}
	empty, far, flood := "", 200.0, "flood"
	patches := map[string]usecase.FieldPatch{
		"empty name": {Name: &empty}, "bad area": {Area: &usecase.AreaInput{ValueMilli: -1, Unit: "decimal"}},
		"bad lng": {Lng: &far}, "bad irrigation": {Irrigation: &flood},
	}
	for name, p := range patches {
		if _, err := (usecase.UpdateField{Deps: d}).Execute(farmer, f.ID, p); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	lat, district := 22.5, "Khulna"
	got, err := usecase.UpdateField{Deps: d}.Execute(farmer, f.ID, usecase.FieldPatch{Lat: &lat, District: &district})
	if err != nil || got.Location.Lat() != 22.5 || got.Location.Lng() != 90 || got.District != "Khulna" {
		t.Fatal(got, err)
	}
	if _, err := (usecase.PutSoil{Deps: d}).Execute(farmer, f.ID, usecase.SoilInput{Texture: "peat"}); !errors.Is(err, domain.ErrInvalidField) {
		t.Fatal(err)
	}
	if _, err := (usecase.AddPlot{Deps: d}).Execute(farmer, f.ID, usecase.PlotInput{Area: usecase.AreaInput{Unit: "x"}}); !errors.Is(err, domain.ErrInvalidArea) {
		t.Fatal(err)
	}
	anon := ctx
	if _, err := (usecase.CreateField{Deps: d}).Execute(anon, input()); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	if _, err := (usecase.ListFields{Deps: d}).Execute(anon, ""); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	if _, err := (usecase.GetField{Deps: d}).Execute(anon, f.ID); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
}

func TestUseCases_StorageFailures(t *testing.T) {
	type run func(d usecase.Deps, id string) error
	create := func(d usecase.Deps, _ string) error {
		_, err := usecase.CreateField{Deps: d}.Execute(farmer, input())
		return err
	}
	update := func(d usecase.Deps, id string) error {
		n := "x"
		_, err := usecase.UpdateField{Deps: d}.Execute(farmer, id, usecase.FieldPatch{Name: &n})
		return err
	}
	del := func(d usecase.Deps, id string) error { return usecase.DeleteField{Deps: d}.Execute(farmer, id) }
	soil := func(d usecase.Deps, id string) error {
		_, err := usecase.PutSoil{Deps: d}.Execute(farmer, id, usecase.SoilInput{Texture: "loam", PH: 6})
		return err
	}
	plot := func(d usecase.Deps, id string) error {
		_, err := usecase.AddPlot{Deps: d}.Execute(farmer, id, usecase.PlotInput{Name: "p", Area: usecase.AreaInput{ValueMilli: 1, Unit: "decimal"}, CropCode: "wheat"})
		return err
	}
	cases := []struct {
		name  string
		fault harness.Fault
		run   run
	}{
		{"create insert", harness.Fault{Op: "exec", Match: "INSERT INTO farm_fields", Err: boom}, create},
		{"create soil", harness.Fault{Op: "exec", Match: "INSERT INTO farm_soil_profiles", Err: boom}, create},
		{"create publish", harness.Fault{Op: "exec", Match: "INSERT INTO outbox", Err: boom}, create},
		{"get plots", harness.Fault{Op: "query", Match: "FROM farm_plots", Err: boom}, update},
		{"update", harness.Fault{Op: "exec", Match: "UPDATE farm_fields", Err: boom}, update},
		{"update publish", harness.Fault{Op: "exec", Match: "INSERT INTO outbox", Err: boom}, update},
		{"delete", harness.Fault{Op: "exec", Match: "DELETE FROM farm_fields", Err: boom}, del},
		{"delete load", harness.Fault{Op: "query", Match: "FROM farm_fields f", Err: boom}, del},
		{"soil", harness.Fault{Op: "exec", Match: "INSERT INTO farm_soil_profiles", Err: boom}, soil},
		{"soil load", harness.Fault{Op: "query", Match: "FROM farm_fields f", Err: boom}, soil},
		{"plot", harness.Fault{Op: "exec", Match: "INSERT INTO farm_plots", Err: boom}, plot},
		{"plot load", harness.Fault{Op: "query", Match: "FROM farm_fields f", Err: boom}, plot},
	}
	for i, c := range cases {
		db := harness.DB(t)
		seed := deps(t, db)
		in := input()
		in.Name = fmt.Sprint("seed-", i)
		f, err := usecase.CreateField{Deps: seed}.Execute(farmer, in)
		if err != nil {
			t.Fatal(err)
		}
		d := deps(t, harness.FaultyOver(db, c.fault))
		if err := c.run(d, f.ID); !errors.Is(err, boom) {
			t.Errorf("%s: want boom, got %v", c.name, err)
		}
	}
}

func TestModule_PublishedViews(t *testing.T) {
	a, m := setup(t, harness.DB(t))
	f := create(t, a, a.Token(owner, authz.Farmer))
	a.Do("POST", "/v1/fields/"+f.ID+"/plots", transport.PlotRequest{Name: "A", Area: transport.AreaIn{ValueMilli: 10, Unit: "decimal"}, CropCode: "potato"},
		a.Token(owner, authz.Farmer)).Expect(t, 201)
	v, err := m.Field(ctx, f.ID)
	if err != nil || v.OwnerID != owner || v.Soil == nil || v.Soil.PHx10 != 62 || v.AreaHectare <= 0.3 || len(v.CropCodes) != 1 {
		t.Fatalf("%+v %v", v, err)
	}
	if _, err := m.Field(ctx, nobody); !errors.Is(err, domain.ErrFieldNotFound) {
		t.Fatal(err)
	}
	f2 := a.Do("POST", "/v1/fields", transport.FieldRequest{Name: "bare", Area: transport.AreaIn{ValueMilli: 1, Unit: "acre"}, Irrigation: "none"}, a.Token(owner, authz.Farmer))
	var bare transport.Field
	f2.Expect(t, 201).Decode(t, &bare)
	if v, _ := m.Field(ctx, bare.ID); v.Soil != nil {
		t.Fatal("no soil test yet")
	}
	crops := m.Crops()
	if len(crops) != 12 || len(crops[0].Textures) == 0 || len(farm.Errors()) != 5 {
		t.Fatal()
	}
}
