package covgate

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

const mod = "example.com/m"

const profile = `mode: atomic
example.com/m/a/a.go:1.1,2.2 2 0
example.com/m/a/a.go:1.1,2.2 2 3
example.com/m/a/a.go:3.1,4.2 1 1
example.com/m/b/b.go:1.1,2.2 4 0
example.com/m/b/b.go:3.1,4.2 4 1

example.com/m/cmd/api/main.go:1.1,2.2 5 0
`

func mustIgnore(t *testing.T, s string) *Matcher {
	t.Helper()
	m, err := ParseIgnore(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestParseProfile_MergesDuplicateBlocksByMax(t *testing.T) {
	blocks, err := ParseProfile(strings.NewReader(profile))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 5 {
		t.Fatalf("want 5 merged blocks, got %d", len(blocks))
	}
	if blocks[0].Count != 3 {
		t.Fatalf("want merged count 3, got %d", blocks[0].Count)
	}
}

func TestParseProfile_RejectsMalformedLines(t *testing.T) {
	cases := []string{
		"nocolon",
		"a.go:1.1,2.2 1",
		"a.go:1.1,2.2 x 1",
		"a.go:1.1,2.2 1 x",
	}
	for _, c := range cases {
		if _, err := ParseProfile(strings.NewReader(c)); err == nil {
			t.Errorf("%q: expected error", c)
		}
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestParseProfile_PropagatesReadError(t *testing.T) {
	if _, err := ParseProfile(errReader{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestMatcher_Globs(t *testing.T) {
	m := mustIgnore(t, "# c\n\ncmd/*/main.go\n**/*_gen.go\n**/mocks/**\napi/index.go\n")
	cases := map[string]bool{
		"cmd/api/main.go":        true,
		"cmd/api/sub/main.go":    false,
		"cmd/api/run.go":         false,
		"x_gen.go":               true,
		"a/b/x_gen.go":           true,
		"a/mocks/fake.go":        true,
		"mocks/fake.go":          true,
		"api/index.go":           true,
		"api/index_go":           false,
		"internal/platform/x.go": false,
	}
	for p, want := range cases {
		if got := m.Match(p); got != want {
			t.Errorf("%s: want %v got %v", p, want, got)
		}
	}
	if len(m.Patterns()) != 4 {
		t.Fatalf("patterns: %v", m.Patterns())
	}
}

func TestSummarizeAndCheck(t *testing.T) {
	blocks, _ := ParseProfile(strings.NewReader(profile))
	res := Summarize(blocks, mod, mustIgnore(t, "cmd/*/main.go"))
	if len(res) != 2 || res[0].Package != "a" || res[0].Percent() != 100 || res[1].Percent() != 50 {
		t.Fatalf("unexpected %+v", res)
	}
	var out bytes.Buffer
	failed := Check(&out, res, 100)
	if len(failed) != 1 || failed[0].Package != "b" || !strings.Contains(out.String(), "FAIL") {
		t.Fatalf("unexpected %v %s", failed, out.String())
	}
	if (PackageResult{}).Percent() != 100 {
		t.Fatal("empty package must be 100%")
	}
	unc := UncoveredBlocks(blocks, mod, mustIgnore(t, "cmd/*/main.go"))
	if len(unc) != 1 || unc[0] != "b/b.go:1.1,2.2" {
		t.Fatalf("uncovered %v", unc)
	}
}

func TestRun_ExitCodes(t *testing.T) {
	good := "mode: set\nexample.com/m/a/a.go:1.1,2.2 1 1\n"
	fsys := fstest.MapFS{
		"good.out":   {Data: []byte(good)},
		"bad.out":    {Data: []byte(profile)},
		"broken.out": {Data: []byte("garbage")},
		".ci":        {Data: []byte("cmd/*/main.go\n")},
	}
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"pass", []string{"-profile", "good.out", "-ignore", ".ci", "-module", mod}, 0},
		{"fail", []string{"-profile", "bad.out", "-ignore", ".ci", "-module", mod}, 1},
		{"fail verbose", []string{"-v", "-profile", "bad.out", "-ignore", ".ci", "-module", mod}, 1},
		{"bad flag", []string{"-nope"}, 2},
		{"missing profile", []string{"-profile", "none"}, 2},
		{"broken profile", []string{"-profile", "broken.out"}, 2},
		{"missing ignore", []string{"-profile", "good.out", "-ignore", "none"}, 2},
	}
	for _, c := range cases {
		var out bytes.Buffer
		if got := Run(c.args, fsys, &out); got != c.want {
			t.Errorf("%s: want %d got %d: %s", c.name, c.want, got, out.String())
		}
	}
}

func TestRun_IgnoreReadError(t *testing.T) {
	fsys := fstest.MapFS{
		"good.out": {Data: []byte("mode: set\n")},
		".ci":      {Data: []byte(strings.Repeat("x", 70*1024))},
	}
	var out bytes.Buffer
	if got := Run([]string{"-profile", "good.out", "-ignore", ".ci"}, fsys, &out); got != 2 {
		t.Fatalf("want 2 got %d", got)
	}
}

// TestCoverageIgnore_IsExactlyThePermittedSet keeps the exclusion list honest:
// only main bodies, generated code, mocks and the Vercel shim may be excluded.
func TestCoverageIgnore_IsExactlyThePermittedSet(t *testing.T) {
	f, err := os.Open("../../../.coverageignore")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m, err := ParseIgnore(f)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cmd/*/main.go", "api/index.go", "**/*_gen.go", "**/*.pb.go", "**/mocks/**"}
	got := m.Patterns()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("coverageignore drifted: %v", got)
	}
}
