package config

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

var secrets = []string{
	"AGRI_DATABASE_URL=postgres://x",
	"AGRI_AUTH_JWT_SECRET=0123456789abcdef0123456789abcdef",
	"AGRI_STORAGE_LOCAL_SIGNING_KEY=0123456789abcdef0123456789abcdef",
}

func load(t *testing.T, src Source) (*Config, *errs.Error) {
	t.Helper()
	src.Environ = append(append([]string(nil), secrets...), src.Environ...)
	cfg, err := Load(src)
	if err != nil {
		e, ok := errs.As(err)
		if !ok {
			t.Fatalf("untyped error %v", err)
		}
		return nil, e
	}
	return cfg, nil
}

func TestLoad_DefaultsApply(t *testing.T) {
	cfg, e := load(t, Source{})
	if e != nil {
		t.Fatal(Report(e))
	}
	if cfg.Auth.AccessTTL != 15*time.Minute || cfg.Localization.DefaultLanguage != "bn" ||
		cfg.AI.MinDiagnosisConfidence != 0.60 || cfg.Auth.Argon2Threads != 2 ||
		!reflect.DeepEqual(cfg.HTTP.CORSAllowedOrigins, []string{"*"}) || !cfg.Features.GuestMode {
		t.Fatalf("%+v", cfg)
	}
}

func TestLoad_PrecedenceDefaultsFileEnvFlags(t *testing.T) {
	fsys := fstest.MapFS{"c.yaml": {Data: []byte("app:\n  name: from-file\nauth:\n  access_ttl: 20m\nhttp:\n  addr: ':1'\n")}}
	cfg, e := load(t, Source{
		FS:      fsys,
		Args:    []string{"-config", "c.yaml", "-set", "http.addr=:3"},
		Environ: []string{"AGRI_AUTH_ACCESS_TTL=30m", "AGRI_HTTP_ADDR=:2", "OTHER=1", "AGRI_NOEQUALS"},
	})
	if e != nil {
		t.Fatal(Report(e))
	}
	if cfg.App.Name != "from-file" || cfg.Auth.AccessTTL != 30*time.Minute || cfg.HTTP.Addr != ":3" {
		t.Fatalf("%+v", cfg)
	}
}

func TestLoad_EmptyFileKeepsDefaults(t *testing.T) {
	cfg, e := load(t, Source{FS: fstest.MapFS{"c.yaml": {Data: nil}}, Args: []string{"-config", "c.yaml"}})
	if e != nil || cfg.App.Name != "agrismart" {
		t.Fatal(e)
	}
}

func TestLoad_Errors(t *testing.T) {
	fsys := fstest.MapFS{
		"bad.yaml":     {Data: []byte("app: [")},
		"secret.yaml":  {Data: []byte("auth:\n  jwt_secret: nope\n")},
		"unknown.yaml": {Data: []byte("app:\n  bogus: 1\n")},
		"scalar.yaml":  {Data: []byte("auth: 5\n")},
	}
	cases := []struct {
		name  string
		src   Source
		field string
	}{
		{"bad flag", Source{Args: []string{"-nope"}}, "flags"},
		{"file disabled", Source{Args: []string{"-config", "x"}}, "config"},
		{"missing file", Source{FS: fsys, Args: []string{"-config", "none"}}, "config"},
		{"bad yaml", Source{FS: fsys, Args: []string{"-config", "bad.yaml"}}, "config"},
		{"secret in file", Source{FS: fsys, Args: []string{"-config", "secret.yaml"}}, "auth.jwt_secret"},
		{"unknown key in file", Source{FS: fsys, Args: []string{"-config", "unknown.yaml"}}, "config"},
		{"scalar section", Source{FS: fsys, Args: []string{"-config", "scalar.yaml"}}, "config"},
		{"bad env", Source{Environ: []string{"AGRI_HTTP_MAX_BODY_BYTES=x"}}, "http.max_body_bytes"},
		{"unknown set", Source{Args: []string{"-set", "nope.x=1"}}, "nope.x"},
		{"secret set", Source{Args: []string{"-set", "database.url=x"}}, "database.url"},
		{"bad set", Source{Args: []string{"-set", "http.trust_proxy_headers=maybe"}}, "http.trust_proxy_headers"},
		{"tag validation", Source{Args: []string{"-set", "app.env=staging"}}, "app.env"},
		{"weights", Source{Args: []string{"-set", "advisory.weight_seed=0.5"}}, "advisory.weights"},
		{"otp in prod", Source{Args: []string{"-set", "app.env=production", "-set", "features.expose_otp=true"}}, "features.expose_otp"},
		{"s3 creds", Source{Args: []string{"-set", "storage.backend=s3"}}, "storage.s3_credentials"},
		{"min>max conns", Source{Args: []string{"-set", "database.min_conns=20"}}, "database.min_conns"},
	}
	for _, c := range cases {
		_, e := load(t, c.src)
		if e == nil || !errors.Is(e, ErrInvalid) {
			t.Errorf("%s: expected config error, got %v", c.name, e)
			continue
		}
		if _, ok := e.Fields[c.field]; !ok {
			t.Errorf("%s: want field %q in %v", c.name, c.field, e.Fields)
		}
	}
}

func TestLoad_LocalSigningKeyRequired(t *testing.T) {
	_, err := Load(Source{Environ: secrets[:2]})
	e, _ := errs.As(err)
	if e == nil || e.Fields["storage.local_signing_key"] == "" {
		t.Fatal(err)
	}
}

func TestSetValue_AllKinds(t *testing.T) {
	var s struct {
		D time.Duration
		L []string
		S string
		B bool
		I int
		U uint32
		F float64
		C complex64
	}
	v := reflect.ValueOf(&s).Elem()
	ok := map[int]string{0: "1s", 1: " a, ,b ", 2: "x", 3: "true", 4: "7", 5: "9", 6: "0.5"}
	for i, raw := range ok {
		if err := setValue(v.Field(i), raw); err != nil {
			t.Fatalf("field %d: %v", i, err)
		}
	}
	if s.D != time.Second || !reflect.DeepEqual(s.L, []string{"a", "b"}) || s.U != 9 || s.F != 0.5 {
		t.Fatalf("%+v", s)
	}
	for i := range []int{0, 3, 4, 5, 6, 7} {
		idx := []int{0, 3, 4, 5, 6, 7}[i]
		if err := setValue(v.Field(idx), "?!"); err == nil {
			t.Errorf("field %d: expected error", idx)
		}
	}
}

func TestDefaults_AllParse(t *testing.T) {
	var s struct {
		N int `yaml:"n" default:"x"`
	}
	fields := fieldsOf(&s)
	if err := applyLayer(fields, defaultsLayer(fields)); err == nil {
		t.Fatal("a malformed default must be reported")
	}
}

func TestReport(t *testing.T) {
	if Report(errors.New("plain")) != "plain" {
		t.Fatal()
	}
	r := Report(ErrInvalid.WithField("b", "2").WithField("a", "1"))
	if r != "invalid configuration:\n  a: 1\n  b: 2" {
		t.Fatal(r)
	}
	var m multiFlag
	_ = m.Set("a")
	if m.String() != "a" {
		t.Fatal()
	}
}

// TestExampleFiles_CoverEveryField keeps config.example.yaml and .env.example honest.
func TestExampleFiles_CoverEveryField(t *testing.T) {
	raw, err := os.ReadFile("../../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var tree map[string]any
	if err := yaml.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	envRaw, err := os.ReadFile("../../../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	env := string(envRaw)
	for _, f := range Fields(&Config{}) {
		if f.Secret {
			if hasPath(tree, f.Path) {
				t.Errorf("%s is secret and must not be in config.example.yaml", f.Path)
			}
		} else if !hasPath(tree, f.Path) {
			t.Errorf("config.example.yaml misses %s", f.Path)
		}
		if !strings.Contains(env, "\n"+f.Env+"=") {
			t.Errorf(".env.example misses %s", f.Env)
		}
	}
	// The example file must itself load cleanly.
	cfg, e := load(t, Source{FS: os.DirFS("../../.."), Args: []string{"-config", "config.example.yaml"}})
	if e != nil || cfg.HTTP.Addr != ":8080" {
		t.Fatal(Report(e))
	}
}
