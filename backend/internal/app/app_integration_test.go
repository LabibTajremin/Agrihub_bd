//go:build integration

package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter/stub"
	diagdomain "github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/domain"
	farmdomain "github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/seed"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/config"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/migrations"
	"github.com/labibtajremin/agrihub_bd/backend/test/harness"
)

var update = flag.Bool("update", false, "rewrite golden files")

var epoch = time.Date(2026, 3, 1, 6, 0, 0, 0, time.UTC)

var logSink io.Writer = io.Discard

func infra() Infra {
	return Infra{Clock: clock.NewFake(epoch), IDs: idgen.UUIDv7{}, Random: rand.Reader, LogOutput: logSink}
}

// migratedEnv returns environment variables for a fresh, migrated database.
func migratedEnv(t *testing.T) []string {
	t.Helper()
	dsn := harness.NewDatabaseDSN(t)
	if err := database.Migrate(dsn, migrations.FS, "up"); err != nil {
		t.Fatal(err)
	}
	return baseEnv(t, dsn)
}

func baseEnv(t *testing.T, dsn string) []string {
	return []string{
		"AGRI_DATABASE_URL=" + dsn,
		"AGRI_AUTH_JWT_SECRET=0123456789abcdef0123456789abcdef",
		"AGRI_STORAGE_LOCAL_SIGNING_KEY=fedcba9876543210fedcba9876543210",
		"AGRI_STORAGE_LOCAL_DIR=" + t.TempDir(),
		"AGRI_FEATURES_EXPOSE_OTP=true",
		"AGRI_AUTH_ARGON2_MEMORY_KIB=64",
		"AGRI_OBSERVABILITY_LOG_LEVEL=error",
		"AGRI_APP_BASE_URL=http://api.test",
	}
}

func build(t *testing.T, environ []string) *App {
	t.Helper()
	cfg, err := config.Load(config.Source{Environ: environ})
	if err != nil {
		t.Fatal(config.Report(err))
	}
	a, err := Build(context.Background(), cfg, infra())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}

type client struct {
	t *testing.T
	h http.Handler
}

func (c client) do(method, path string, body any, token string, headers ...string) (int, []byte) {
	c.t.Helper()
	var r io.Reader = http.NoBody
	switch b := body.(type) {
	case nil:
	case []byte:
		r = bytes.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		r = bytes.NewReader(raw)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, r)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func (c client) json(method, path string, body any, token string, want int, out any) {
	c.t.Helper()
	code, raw := c.do(method, path, body, token)
	if code != want {
		c.t.Fatalf("%s %s: want %d got %d: %s", method, path, want, code, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatal(err)
		}
	}
}

func leaf() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 48, 36))
	for y := range 36 {
		for x := range 48 {
			img.Set(x, y, color.RGBA{uint8(200 - x*3), uint8(80 + y*2), 30, 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// TestEndToEnd_FirstScanToAlertAndAdvice drives the assembled app like the
// mobile client: guest → upload → diagnose → sign in (upgrade) → field →
// advice → alerts.
func TestEndToEnd_FirstScanToAlertAndAdvice(t *testing.T) {
	a := build(t, migratedEnv(t))
	c := client{t: t, h: a.Router}
	ctx := context.Background()
	if _, err := a.Localization.SeedBundled(ctx); err != nil {
		t.Fatal(err)
	}
	c.json("GET", "/healthz", nil, "", 200, nil)
	var ready Health
	c.json("GET", "/readyz", nil, "", 200, &ready)
	var langs struct{ Languages []map[string]any }
	c.json("GET", "/v1/i18n/languages", nil, "", 200, &langs)
	if len(langs.Languages) != 7 {
		t.Fatal(langs)
	}

	var guest struct {
		AccessToken string `json:"access_token"`
		User        struct{ ID string }
	}
	c.json("POST", "/v1/auth/guest", nil, "", 201, &guest)

	body := leaf()
	sum := sha256.Sum256(body)
	var ticket struct {
		MediaID   string `json:"media_id"`
		UploadURL string `json:"upload_url"`
	}
	c.json("POST", "/v1/media/tickets", map[string]any{"content_type": "image/png", "size_bytes": len(body), "checksum_sha256": hex.EncodeToString(sum[:])}, guest.AccessToken, 201, &ticket)
	u, _ := url.Parse(ticket.UploadURL)
	if code, raw := c.do("PUT", u.RequestURI(), body, "", "Content-Type", "image/png"); code != 204 {
		t.Fatal(code, string(raw))
	}
	c.json("POST", "/v1/media/"+ticket.MediaID+"/complete", nil, guest.AccessToken, 200, nil)
	var scan struct {
		ID        string
		Status    string
		Diagnosis *struct {
			DiseaseCode string `json:"disease_code"`
		}
	}
	c.json("POST", "/v1/scans", map[string]any{"media_id": ticket.MediaID, "crop_code": "rice_aman", "idempotency_key": "first-scan"}, guest.AccessToken, 201, &scan)
	if scan.Diagnosis == nil || (scan.Status != "completed" && scan.Status != "low_confidence") {
		t.Fatalf("%+v", scan)
	}

	var otp struct {
		DevCode string `json:"dev_code"`
	}
	c.json("POST", "/v1/auth/otp/request", map[string]string{"phone": "01711000000"}, guest.AccessToken, 202, &otp)
	var session struct {
		AccessToken string `json:"access_token"`
		User        struct{ ID, Role string }
	}
	c.json("POST", "/v1/auth/otp/verify", map[string]string{"phone": "01711000000", "code": otp.DevCode}, guest.AccessToken, 200, &session)
	if session.User.ID != guest.User.ID || session.User.Role != "farmer" {
		t.Fatal("the guest is upgraded in place, keeping the scan", session.User)
	}
	tok := session.AccessToken
	var history struct{ Scans []map[string]any }
	c.json("GET", "/v1/scans", nil, tok, 200, &history)
	if len(history.Scans) != 1 {
		t.Fatal(history)
	}
	var inbox struct {
		Alerts []struct{ Kind string }
	}
	c.json("GET", "/v1/alerts", nil, tok, 200, &inbox)
	wantFollowup := scan.Status == "completed" && scan.Diagnosis.DiseaseCode != "healthy"
	if (len(inbox.Alerts) == 1) != wantFollowup {
		t.Fatalf("follow-up alert raised through the outbox after the scan: %+v (scan %+v)", inbox, scan)
	}

	var field struct{ ID string }
	c.json("POST", "/v1/fields", map[string]any{"name": "Home", "area": map[string]any{"value_milli": 1000, "unit": "bigha"},
		"location": map[string]any{"lat": 24.85, "lng": 89.37}, "irrigation": "partial",
		"soil": map[string]any{"texture": "clay_loam", "ph": 6.2, "nitrogen_kg_ha": 80}}, tok, 201, &field)
	c.json("POST", "/v1/fields/"+field.ID+"/plots", map[string]any{"name": "A", "area": map[string]any{"value_milli": 500, "unit": "bigha"}, "crop_code": "rice_aman"}, tok, 201, nil)
	c.json("PUT", "/v1/alerts/subscription", map[string]any{"lat": 24.85, "lng": 89.37, "kinds": []string{"heavy_rain", "heat", "blast_risk"}}, tok, 200, nil)
	var recs struct {
		Items []struct {
			CropCode string `json:"crop_code"`
		}
		Outlook struct{ Stale bool }
	}
	c.json("GET", "/v1/advisory/fields/"+field.ID+"/recommendations?season=aman", nil, tok, 200, &recs)
	if len(recs.Items) == 0 || recs.Outlook.Stale {
		t.Fatalf("%+v", recs)
	}
	var rot struct{ Steps []map[string]any }
	c.json("GET", "/v1/advisory/fields/"+field.ID+"/rotation?seasons=3", nil, tok, 200, &rot)
	if len(rot.Steps) != 3 {
		t.Fatal(rot)
	}
	var fc struct {
		Days []map[string]any
	}
	c.json("GET", "/v1/weather?lat=24.85&lng=89.37", nil, tok, 200, &fc)
	if len(fc.Days) != 7 {
		t.Fatal(fc)
	}
	var ans struct {
		AnswerKey string `json:"answer_key"`
	}
	c.json("POST", "/v1/assistant/ask", map[string]string{"lang": "en", "transcript": "what should I plant?"}, tok, 200, &ans)
	if ans.AnswerKey != "voice.answer.crop" {
		t.Fatal(ans)
	}
	admin, _, _ := a.Signer.Issue(authn.Principal{Subject: session.User.ID, Role: authz.Admin})
	var m MetricsSnapshot
	c.json("GET", "/v1/admin/metrics", nil, admin, 200, &m)
	if len(m.Series) == 0 {
		t.Fatal("metrics are recorded")
	}
	c.json("GET", "/v1/admin/metrics", nil, tok, 403, nil)
}

// TestOpenAPI_UpToDate diffs the committed spec against the one generated from
// the route table (run `make openapi` after changing routes or DTOs).
func TestOpenAPI_UpToDate(t *testing.T) {
	a := build(t, migratedEnv(t))
	doc, err := OpenAPI(a.Router.Routes())
	if err != nil {
		t.Fatal(err)
	}
	const path = "../../openapi/openapi.yaml"
	if *update {
		if err := os.WriteFile(path, doc, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(want, doc) {
		t.Fatal("openapi/openapi.yaml is out of date; run `make openapi`")
	}
	// every route is documented with its method
	for _, r := range a.Router.Routes() {
		p := strings.ReplaceAll(r.Path, "...}", "}")
		if !bytes.Contains(doc, []byte("\n    "+p+":\n")) && !bytes.Contains(doc, []byte("\n    "+p+":")) {
			t.Errorf("missing path %s", p)
		}
	}
}

// TestErrorCatalogue_GoldenAndTranslated pins every client-facing error and
// requires its message key in all seven dictionaries.
func TestErrorCatalogue_GoldenAndTranslated(t *testing.T) {
	var b strings.Builder
	dicts, err := seed.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ErrorCatalogue() {
		fmt.Fprintf(&b, "%-32s %-13s %d\n", e.Code, e.Kind, httpx.Status(e.Kind))
		for lang, d := range dicts {
			if d[e.Message] == "" {
				t.Errorf("%s: missing %s", lang, e.Message)
			}
		}
	}
	const path = "testdata/error_catalogue.golden"
	if *update {
		_ = os.MkdirAll("testdata", 0o750)
		if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, _ := os.ReadFile(path)
	if string(want) != b.String() {
		t.Fatalf("error catalogue drifted (run with -update):\n%s", b.String())
	}
}

// TestDictionaries_CoverKeysEmittedByTheBackend checks the i18n keys the API
// returns (diseases, treatments, alerts, narration, assistant, crops).
func TestDictionaries_CoverKeysEmittedByTheBackend(t *testing.T) {
	var keys []string
	for _, d := range stub.Diseases {
		keys = append(keys, "disease."+d+".name", "disease."+d+".description")
		for _, p := range diagdomain.Plans(d) {
			keys = append(keys, p.Steps...)
			if p.SafetyKey != "" {
				keys = append(keys, p.SafetyKey)
			}
		}
	}
	for _, k := range []string{"heavy_rain", "heat", "blast_risk", "disease_followup"} {
		keys = append(keys, "alerts."+k+".title", "alerts."+k+".body")
	}
	for _, k := range stub.ChartKinds {
		keys = append(keys, "narration.chart."+k)
	}
	for _, k := range []string{"weather", "disease", "crop"} {
		keys = append(keys, "voice.answer."+k, "voice.suggestion."+k)
	}
	keys = append(keys, "voice.not_understood")
	for _, c := range farmdomain.Catalogue() {
		keys = append(keys, "crop."+c.Code+".name")
	}
	dicts, _ := seed.Load()
	for lang, d := range dicts {
		for _, k := range keys {
			if d[k] == "" {
				t.Errorf("%s: missing %s", lang, k)
			}
		}
	}
}

func TestRunAPI_ServesAndShutsDownGracefully(t *testing.T) {
	env := Env{Args: []string{"-set", "http.addr=127.0.0.1:0"}, Environ: migratedEnv(t), Stdout: io.Discard, Infra: infra()}
	ctx, cancel := context.WithCancel(context.Background())
	status := make(chan int, 1)
	var addr net.Addr
	ready := make(chan struct{})
	go func() {
		status <- RunAPI(ctx, env, ServeOptions{Ready: func(ln net.Listener) { addr = ln.Addr(); close(ready) }})
	}()
	<-ready
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr.String()+"/healthz", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	cancel()
	if code := <-status; code != ExitOK {
		t.Fatal(code)
	}
}

func TestRunAPI_Failures(t *testing.T) {
	ctx := context.Background()
	out := &bytes.Buffer{}
	if code := RunAPI(ctx, Env{Stdout: out}, ServeOptions{}); code != ExitConfig || !strings.Contains(out.String(), "invalid configuration") {
		t.Fatal(code, out.String())
	}
	bad := baseEnv(t, "postgres://agri:agri@127.0.0.1:1/x?connect_timeout=1")
	if code := RunAPI(ctx, Env{Environ: bad, Stdout: io.Discard, Infra: infra()}, ServeOptions{}); code != ExitFailed {
		t.Fatal("unreachable database", code)
	}
	env := migratedEnv(t)
	if code := RunAPI(ctx, Env{Args: []string{"-set", "http.addr=256.0.0.1:99999"}, Environ: env, Stdout: io.Discard, Infra: infra()}, ServeOptions{}); code != ExitFailed {
		t.Fatal("bad listen address", code)
	}
	closing := ServeOptions{Ready: func(ln net.Listener) { _ = ln.Close() }}
	if code := RunAPI(ctx, Env{Args: []string{"-set", "http.addr=127.0.0.1:0"}, Environ: env, Stdout: io.Discard, Infra: infra()}, closing); code != ExitFailed {
		t.Fatal("serve failure", code)
	}
	if DefaultInfra().Clock == nil {
		t.Fatal()
	}
}

func TestOutboxWorker_TicksAndLogsFailures(t *testing.T) {
	a := build(t, migratedEnv(t))
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() { a.runOutboxWorker(ctx, ticks); close(done) }()
	ticks <- time.Time{}
	a.Close() // pool closed: the next flush fails and is logged
	ticks <- time.Time{}
	ticks <- time.Time{} // accepted only after the previous (failing) flush was fully handled
	cancel()
	<-done
}

func TestReadiness_AndFlushFailureWhenDatabaseIsGone(t *testing.T) {
	a := build(t, migratedEnv(t))
	a.Close()
	c := client{t: t, h: a.Router}
	var h Health
	c.json("GET", "/readyz", nil, "", 503, &h)
	if h.Status != errNotReady.Error() {
		t.Fatal(h)
	}
}

func TestRunMigrateAndSeed(t *testing.T) {
	dsn := harness.NewDatabaseDSN(t)
	env := baseEnv(t, dsn)
	out := &bytes.Buffer{}
	run := func(args ...string) int { return RunMigrate(Env{Args: args, Environ: env, Stdout: out}) }
	if run() != ExitConfig || run("sideways") != ExitConfig {
		t.Fatal("usage")
	}
	if code := RunMigrate(Env{Args: []string{"up"}, Stdout: out}); code != ExitConfig {
		t.Fatal("config error", code)
	}
	// seeding an unmigrated database fails after a successful build
	if code := RunSeed(context.Background(), Env{Environ: env, Stdout: out, Infra: infra()}); code != ExitFailed {
		t.Fatal("seed before migrate", code)
	}
	if run("up") != ExitOK || run("down") != ExitOK || run("up") != ExitOK {
		t.Fatal(out.String())
	}
	out.Reset()
	if code := RunSeed(context.Background(), Env{Environ: env, Stdout: out, Infra: infra()}); code != ExitOK || !strings.Contains(out.String(), "seed bn:") {
		t.Fatal(code, out.String())
	}
	if code := RunSeed(context.Background(), Env{Stdout: out}); code != ExitConfig {
		t.Fatal(code)
	}
	bad := baseEnv(t, "postgres://agri:agri@127.0.0.1:1/x?connect_timeout=1")
	if RunMigrate(Env{Args: []string{"up"}, Environ: bad, Stdout: out}) != ExitFailed {
		t.Fatal("migrate against an unreachable database")
	}
	if RunSeed(context.Background(), Env{Environ: bad, Stdout: out, Infra: infra()}) != ExitFailed {
		t.Fatal("seed against an unreachable database")
	}
}

func TestNewHandler_ServerlessEntrypoint(t *testing.T) {
	h, err := NewHandler(context.Background(), migratedEnv(t), infra())
	if err != nil {
		t.Fatal(err)
	}
	client{t: t, h: h}.json("GET", "/healthz", nil, "", 200, nil)
	if _, err := NewHandler(context.Background(), nil, infra()); err == nil {
		t.Fatal("missing configuration")
	}
	if _, err := NewHandler(context.Background(), baseEnv(t, "postgres://a:b@127.0.0.1:1/x?connect_timeout=1"), infra()); err == nil {
		t.Fatal("unreachable database")
	}
}

func TestBuild_Variants(t *testing.T) {
	env := migratedEnv(t)
	mr := miniredis.RunT(t)
	a := build(t, append(append([]string(nil), env...), "AGRI_REDIS_URL=redis://"+mr.Addr()))
	client{t: t, h: a.Router}.json("GET", "/healthz", nil, "", 200, nil)

	load := func(extra ...string) *config.Config {
		cfg, err := config.Load(config.Source{Environ: append(append([]string(nil), env...), extra...)})
		if err != nil {
			t.Fatal(config.Report(err))
		}
		return cfg
	}
	cfg := load("AGRI_REDIS_URL=not-a-url")
	if _, err := Build(context.Background(), cfg, infra()); err == nil {
		t.Fatal("invalid redis url")
	}
	cfg = load()
	cfg.AI.Provider = "gpt" // unregistered provider
	if _, err := Build(context.Background(), cfg, infra()); err == nil {
		t.Fatal("unknown AI provider must fail at boot")
	}
}
