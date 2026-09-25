package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// EnvPrefix prefixes every derived environment variable name.
const EnvPrefix = "AGRI_"

// ErrInvalid is returned when configuration fails validation.
var ErrInvalid = errs.Validation("config.invalid")

// Source bundles every input of Load so it is fully injectable.
type Source struct {
	Args    []string // command-line flags (without program name)
	Environ []string // KEY=VALUE pairs
	// ReadFile reads the -config path (os.ReadFile in production); nil disables file loading.
	ReadFile func(name string) ([]byte, error)
}

// Field describes one leaf of the config tree.
type Field struct {
	Path   string // dotted yaml path, e.g. auth.access_ttl
	Env    string // AGRI_AUTH_ACCESS_TTL
	Secret bool
	Value  reflect.Value
	Tag    reflect.StructTag
}

// Fields enumerates every leaf of cfg in declaration order.
func Fields(cfg *Config) []Field { return fieldsOf(cfg) }

func fieldsOf(ptr any) []Field {
	var out []Field
	walk(reflect.ValueOf(ptr).Elem(), nil, &out)
	return out
}

// defaultsLayer expresses struct defaults as an env-shaped layer so defaults and
// environment share one parsing path.
func defaultsLayer(fields []Field) map[string]string {
	out := map[string]string{}
	for _, f := range fields {
		if def, ok := f.Tag.Lookup("default"); ok {
			out[f.Env] = def
		}
	}
	return out
}

func applyLayer(fields []Field, layer map[string]string) error {
	for _, f := range fields {
		if raw, ok := layer[f.Env]; ok {
			if err := setValue(f.Value, raw); err != nil {
				return ErrInvalid.Wrap(err).WithField(f.Path, "invalid value for "+f.Env)
			}
		}
	}
	return nil
}

func walk(v reflect.Value, prefix []string, out *[]Field) {
	t := v.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		name := strings.Split(sf.Tag.Get("yaml"), ",")[0]
		path := append(append([]string(nil), prefix...), name)
		fv := v.Field(i)
		if sf.Type.Kind() == reflect.Struct {
			walk(fv, path, out)
			continue
		}
		*out = append(*out, Field{
			Path:   strings.Join(path, "."),
			Env:    EnvPrefix + strings.ToUpper(strings.Join(path, "_")),
			Secret: sf.Tag.Get("secret") == "true",
			Value:  fv,
			Tag:    sf.Tag,
		})
	}
}

// Load builds and validates the configuration from src.
func Load(src Source) (*Config, error) {
	fl := flag.NewFlagSet("agrismart", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	file := fl.String("config", "", "path to config.yaml")
	var sets multiFlag
	fl.Var(&sets, "set", "override a key: -set section.key=value (repeatable)")
	if err := fl.Parse(src.Args); err != nil {
		return nil, ErrInvalid.Wrap(err).WithField("flags", err.Error())
	}

	cfg := &Config{}
	fields := Fields(cfg)
	steps := []func() error{
		func() error { return applyLayer(fields, defaultsLayer(fields)) },
		func() error { return loadFile(src.ReadFile, *file, cfg, fields) },
		func() error { return applyLayer(fields, parseEnv(src.Environ)) },
		func() error { return applySets(fields, sets) },
		func() error { return Validate(cfg) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func applySets(fields []Field, sets []string) error {
	byPath := make(map[string]Field, len(fields))
	for _, f := range fields {
		byPath[f.Path] = f
	}
	for _, s := range sets {
		key, raw, _ := strings.Cut(s, "=")
		f, ok := byPath[key]
		if !ok {
			return ErrInvalid.WithField(key, "unknown key")
		}
		if f.Secret {
			return ErrInvalid.WithField(key, "secret must come from env")
		}
		if err := setValue(f.Value, raw); err != nil {
			return ErrInvalid.Wrap(err).WithField(key, "invalid value")
		}
	}
	return nil
}

func loadFile(read func(string) ([]byte, error), name string, cfg *Config, fields []Field) error {
	if name == "" {
		return nil
	}
	if read == nil {
		return ErrInvalid.WithField("config", "file loading disabled")
	}
	raw, err := read(name)
	if err != nil {
		return ErrInvalid.Wrap(err).WithField("config", "unreadable")
	}
	var tree map[string]any
	if err := yaml.Unmarshal(raw, &tree); err != nil {
		return ErrInvalid.Wrap(err).WithField("config", "invalid yaml")
	}
	for _, f := range fields {
		if f.Secret && hasPath(tree, f.Path) {
			return ErrInvalid.WithField(f.Path, "secret must come from env")
		}
	}
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return ErrInvalid.Wrap(err).WithField("config", err.Error())
	}
	return nil
}

func hasPath(tree map[string]any, path string) bool {
	var cur any = tree
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		if cur, ok = m[part]; !ok {
			return false
		}
	}
	return true
}

func parseEnv(environ []string) map[string]string {
	out := map[string]string{}
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(k, EnvPrefix) {
			out[k] = v
		}
	}
	return out
}

func setValue(v reflect.Value, raw string) error {
	raw = strings.TrimSpace(raw)
	switch v.Interface().(type) {
	case time.Duration:
		d, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		v.SetInt(int64(d))
		return nil
	case []string:
		var items []string
		for _, s := range strings.Split(raw, ",") {
			if s = strings.TrimSpace(s); s != "" {
				items = append(items, s)
			}
		}
		v.Set(reflect.ValueOf(items))
		return nil
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(n)
	case reflect.Uint8, reflect.Uint32:
		n, err := strconv.ParseUint(raw, 10, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetUint(n)
	case reflect.Float64:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return err
		}
		v.SetFloat(f)
	default:
		return fmt.Errorf("unsupported kind %s", v.Kind())
	}
	return nil
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(s string) error { *m = append(*m, s); return nil }

// WeightTolerance is the allowed float error when summing advisory weights.
const WeightTolerance = 1e-9

// Validate checks struct tags plus cross-field invariants and returns a
// field-by-field report.
func Validate(cfg *Config) error {
	out := ErrInvalid
	failed := false
	if err := validator.New().Struct(cfg); err != nil {
		if e, ok := errs.As(err); ok {
			for k, v := range e.Fields {
				out = out.WithField(k, v)
			}
		}
		failed = true
	}
	a := cfg.Advisory
	if sum := a.WeightSoil + a.WeightWater + a.WeightPest + a.WeightMarket + a.WeightSeed; math.Abs(sum-1) > WeightTolerance {
		out = out.WithField("advisory.weights", fmt.Sprintf("must sum to 1.0, got %.4f", sum))
		failed = true
	}
	if cfg.App.Env == "production" && cfg.Features.ExposeOTP {
		out = out.WithField("features.expose_otp", "forbidden in production")
		failed = true
	}
	if cfg.Storage.Backend == "local" && len(cfg.Storage.LocalSigningKey) < 32 {
		out = out.WithField("storage.local_signing_key", "required (min 32 chars) for local backend")
		failed = true
	}
	if cfg.Storage.Backend == "s3" && (cfg.Storage.S3AccessKey == "" || cfg.Storage.S3SecretKey == "") {
		out = out.WithField("storage.s3_credentials", "required for s3 backend")
		failed = true
	}
	if cfg.Database.MinConns > cfg.Database.MaxConns {
		out = out.WithField("database.min_conns", "must not exceed max_conns")
		failed = true
	}
	if failed {
		return out
	}
	return nil
}

// Report renders a validation error as sorted "field: problem" lines.
func Report(err error) string {
	e, ok := errs.As(err)
	if !ok {
		return err.Error()
	}
	lines := make([]string, 0, len(e.Fields))
	for k, v := range e.Fields {
		lines = append(lines, "  "+k+": "+v)
	}
	sort.Strings(lines)
	return "invalid configuration:\n" + strings.Join(lines, "\n")
}
