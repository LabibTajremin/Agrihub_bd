package authz

import (
	"errors"
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestMatrix_Golden(t *testing.T) {
	got := Render()
	const path = "testdata/matrix.golden"
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatalf("permission matrix drifted; run go test ./internal/platform/authz -update\n%s", got)
	}
}

func TestRoles_AreStrictlyNested(t *testing.T) {
	roles := Roles()
	for i := 1; i < len(roles); i++ {
		for _, p := range PermissionsOf(roles[i-1]) {
			if !Can(roles[i], p) {
				t.Errorf("%s lacks %s held by %s", roles[i], p, roles[i-1])
			}
		}
		if len(PermissionsOf(roles[i])) <= len(PermissionsOf(roles[i-1])) {
			t.Errorf("%s must be strictly more privileged than %s", roles[i], roles[i-1])
		}
	}
	if len(PermissionsOf(Admin)) != len(Permissions()) {
		t.Fatal("admin holds everything")
	}
}

func TestCheckAndValidRole(t *testing.T) {
	if !ValidRole(Farmer) || ValidRole("root") {
		t.Fatal()
	}
	if err := Check(Guest, DictionaryWrite); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if err := Check(Admin, DictionaryWrite); err != nil {
		t.Fatal(err)
	}
	if Can("root", ScanRead) {
		t.Fatal("unknown role has nothing")
	}
}

// TestOwned_EveryRoleActionOwnership table-tests the ownership policy for every
// role × (read, write) × (owner, non-owner, anonymous) combination.
func TestOwned_EveryRoleActionOwnership(t *testing.T) {
	type action struct{ own, any Permission }
	actions := map[string]action{"field.read": {FieldRead, FieldReadAny}, "field.write": {FieldWrite, FieldWriteAny}, "scan.read": {ScanRead, ScanReadAny}}
	want := map[Role]map[string][3]bool{ // [owner, other, empty subject]
		Guest:        {"field.read": {false, false, false}, "field.write": {false, false, false}, "scan.read": {true, false, false}},
		Farmer:       {"field.read": {true, false, false}, "field.write": {true, false, false}, "scan.read": {true, false, false}},
		FieldOfficer: {"field.read": {true, true, true}, "field.write": {true, false, false}, "scan.read": {true, true, true}},
		Agronomist:   {"field.read": {true, true, true}, "field.write": {true, false, false}, "scan.read": {true, true, true}},
		Admin:        {"field.read": {true, true, true}, "field.write": {true, true, true}, "scan.read": {true, true, true}},
	}
	for role, byAction := range want {
		for name, a := range actions {
			exp := byAction[name]
			subjects := [3]string{"u1", "u2", ""}
			for i, subject := range subjects {
				got := Owned(role, subject, "u1", a.own, a.any) == nil
				if got != exp[i] {
					t.Errorf("%s %s subject=%q: want %v", role, name, subject, exp[i])
				}
			}
		}
	}
}
