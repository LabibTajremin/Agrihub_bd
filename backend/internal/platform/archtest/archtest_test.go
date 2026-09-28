package archtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/labibtajremin/agrihub_bd/backend"

// TestArchitecture_RealTree is the gate: the actual codebase must obey every law.
func TestArchitecture_RealTree(t *testing.T) {
	vs, err := Check("../../..", modulePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		t.Error(v)
	}
}

// TestArchitecture_PlantedViolationsAreCaught proves the gate bites: each rule
// is violated once in testdata/planted and must be reported.
func TestArchitecture_PlantedViolationsAreCaught(t *testing.T) {
	vs, err := Check("testdata/planted", "example.com/m")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, v := range vs {
		got[v.Rule]++
	}
	want := map[string]int{
		"domain-purity":    2, // pgx + identity/domain
		"module-isolation": 1, // farm -> identity (aiadapter port is allowed)
		"platform-pure":    2, // app + modules
		"no-sql-usecase":   2, // pgx + platform/database
		"transport-repo":   1,
		"no-time-now":      1, // aliased time in farm/domain; clock and tests exempt
		"no-init":          1, // method named init is fine
		"no-panic":         1,
	}
	for rule, n := range want {
		if got[rule] != n {
			t.Errorf("%s: want %d got %d (%v)", rule, n, got[rule], vs)
		}
	}
	if !strings.HasPrefix(vs[0].String(), vs[0].Rule+": ") {
		t.Fatal(vs[0].String())
	}
}

// TestArchitecture_RemovingViolationPasses shows the same fixture passes once
// the violating files are removed.
func TestArchitecture_RemovingViolationPasses(t *testing.T) {
	dir := t.TempDir()
	clean := filepath.Join(dir, "internal/modules/farm/domain")
	if err := os.MkdirAll(clean, 0o750); err != nil {
		t.Fatal(err)
	}
	src := "package domain\n\nimport (\n\t\"time\"\n\n\t\"example.com/m/internal/platform/errs\"\n)\n\nvar _ = errs.New\n\nfunc Age(now time.Time) time.Duration { return now.Sub(now) }\n"
	if err := os.WriteFile(filepath.Join(clean, "ok.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	vs, err := Check(dir, "example.com/m")
	if err != nil || len(vs) != 0 {
		t.Fatal(vs, err)
	}
}

func TestCheck_Errors(t *testing.T) {
	if _, err := Check("does-not-exist", "m"); err == nil {
		t.Fatal("missing root")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte("package"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(dir, "m"); err == nil {
		t.Fatal("parse error must surface")
	}
}
