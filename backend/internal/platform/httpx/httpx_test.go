package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/metrics"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/ratelimit"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

type ctxSubject struct{}

type fakeAuthn struct{}

func (fakeAuthn) Authenticate(r *http.Request) (context.Context, error) {
	switch r.Header.Get("Authorization") {
	case "Bearer bad":
		return nil, errs.Unauthorized("auth.invalid_token")
	case "Bearer admin":
		SetSubject(r.Context(), "admin")
		return context.WithValue(r.Context(), ctxSubject{}, "admin"), nil
	}
	return r.Context(), nil
}

type fakeAuthz struct{}

func (fakeAuthz) Authorize(ctx context.Context, perm string) error {
	if ctx.Value(ctxSubject{}) == "admin" {
		return nil
	}
	return errs.Forbidden("auth.forbidden")
}

type errLimiter struct{}

func (errLimiter) Allow(context.Context, string, ratelimit.Rate) (ratelimit.Decision, error) {
	return ratelimit.Decision{}, errors.New("down")
}

type harness struct {
	router  *Router
	logs    *bytes.Buffer
	metrics *metrics.Registry
	after   int
}

func newHarness(t *testing.T, limiter ratelimit.Limiter) *harness {
	t.Helper()
	h := &harness{logs: &bytes.Buffer{}, metrics: metrics.NewRegistry()}
	c := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if limiter == nil {
		limiter = ratelimit.NewMemory(c)
	}
	h.router = NewRouter(Options{
		Logger:         slog.New(slog.NewJSONHandler(h.logs, nil)),
		Clock:          c,
		IDs:            &idgen.Sequence{},
		Metrics:        h.metrics,
		Limiter:        limiter,
		Rates:          map[string]ratelimit.Rate{ClassDefault: ratelimit.PerMinute(100), ClassAuth: ratelimit.PerMinute(1)},
		RequestTimeout: time.Second,
		CORSOrigins:    []string{"https://app.example"},
		TrustProxy:     true,
		MaxBodyBytes:   64,
		Authn:          fakeAuthn{},
		Authz:          fakeAuthz{},
		After:          func(*http.Request) { h.after++ },
	})
	v := validator.New()
	type body struct {
		Name string `json:"name" validate:"required"`
	}
	err := h.router.Mount(
		Route{Method: http.MethodGet, Path: "/ok", Handler: func(w http.ResponseWriter, r *http.Request) error {
			if _, ok := r.Context().Deadline(); !ok {
				t.Error("timeout middleware must set a deadline")
			}
			return JSON(w, http.StatusOK, map[string]string{"ip": ClientIP(r.Context())})
		}},
		Route{Method: http.MethodPost, Path: "/echo", Handler: func(w http.ResponseWriter, r *http.Request) error {
			var b body
			if err := Decode(r, &b, v); err != nil {
				return err
			}
			return JSON(w, http.StatusCreated, b)
		}},
		Route{Method: http.MethodGet, Path: "/admin", Permission: "x:y", Handler: func(w http.ResponseWriter, r *http.Request) error {
			return NoContent(w)
		}},
		Route{Method: http.MethodPost, Path: "/login", Class: ClassAuth, Handler: func(w http.ResponseWriter, r *http.Request) error {
			return NoContent(w)
		}},
		Route{Method: http.MethodGet, Path: "/boom", Handler: func(w http.ResponseWriter, r *http.Request) error {
			return errors.New("secret db detail")
		}},
		Route{Method: http.MethodGet, Path: "/panic", Handler: func(w http.ResponseWriter, r *http.Request) error {
			panic("kaboom")
		}},
		Route{Method: http.MethodGet, Path: "/raw", Handler: func(w http.ResponseWriter, r *http.Request) error {
			_, err := w.Write([]byte("raw"))
			return err
		}},
		Route{Method: http.MethodGet, Path: "/silent", Handler: func(w http.ResponseWriter, r *http.Request) error {
			return nil
		}},
		Route{Method: http.MethodPost, Path: "/big", MaxBodyBytes: 1 << 10, Handler: func(w http.ResponseWriter, r *http.Request) error {
			var b body
			return Decode(r, &b, v)
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *harness) do(method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "10.0.0.1:1234"
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

func errBody(t *testing.T, rec *httptest.ResponseRecorder) ErrorDetail {
	t.Helper()
	var b ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("not an error body: %s", rec.Body.String())
	}
	return b.Error
}

func TestRouter_HappyPathAndRequestID(t *testing.T) {
	h := newHarness(t, nil)
	rec := h.do("GET", "/ok", "", map[string]string{"X-Forwarded-For": "1.2.3.4, 5.6.7.8"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "1.2.3.4") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec.Header().Get(HeaderRequestID) != "00000000-0000-7000-8000-000000000001" {
		t.Fatal(rec.Header())
	}
	rec = h.do("GET", "/ok", "", map[string]string{HeaderRequestID: "client-id_1", "X-Real-IP": "9.9.9.9"})
	if rec.Header().Get(HeaderRequestID) != "client-id_1" || !strings.Contains(rec.Body.String(), "9.9.9.9") {
		t.Fatal(rec.Header(), rec.Body.String())
	}
	rec = h.do("GET", "/ok", "", map[string]string{HeaderRequestID: "bad id with spaces"})
	if rec.Header().Get(HeaderRequestID) == "bad id with spaces" {
		t.Fatal("malformed inbound id must be replaced")
	}
	if !strings.Contains(h.logs.String(), `"route":"GET /ok"`) || h.after != 3 {
		t.Fatal(h.logs.String(), h.after)
	}
	if s := h.metrics.Snapshot(); len(s) == 0 || s[0].Route != "GET /ok" {
		t.Fatal(s)
	}
}

func TestRouter_ErrorMappingAndNoLeak(t *testing.T) {
	h := newHarness(t, nil)
	rec := h.do("GET", "/boom", "", nil)
	d := errBody(t, rec)
	if rec.Code != 500 || d.Code != "internal" || strings.Contains(rec.Body.String(), "secret") || d.RequestID == "" {
		t.Fatal(rec.Body.String())
	}
	if !strings.Contains(h.logs.String(), "secret db detail") {
		t.Fatal("cause must be logged")
	}
	rec = h.do("GET", "/panic", "", nil)
	if rec.Code != 500 || errBody(t, rec).Code != "internal" {
		t.Fatal(rec.Body.String())
	}
	rec = h.do("GET", "/nope", "", nil)
	if rec.Code != 404 || errBody(t, rec).Message != "errors.route.not_found" {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if !strings.Contains(h.logs.String(), `"route":"/"`) {
		t.Fatal(h.logs.String())
	}
}

func TestRouter_UnmatchedRouteLabel(t *testing.T) {
	var logs bytes.Buffer
	m := metrics.NewRegistry()
	h := Logger(slog.New(slog.NewJSONHandler(&logs, nil)), clock.System{}, m)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if m.Snapshot()[0].Route != "unmatched" {
		t.Fatal(m.Snapshot())
	}
}

func TestRouter_DecodeAndValidation(t *testing.T) {
	h := newHarness(t, nil)
	cases := []struct {
		body, code string
		status     int
	}{
		{`{"name":"a"}`, "", 201},
		{`{"name":""}`, "request.invalid", 400},
		{`{"nope":1}`, "request.malformed", 400},
		{`{`, "request.malformed", 400},
		{`{"name":"` + strings.Repeat("x", 100) + `"}`, "request.too_large", 400},
	}
	for _, c := range cases {
		rec := h.do("POST", "/echo", c.body, nil)
		if rec.Code != c.status {
			t.Errorf("%s: %d %s", c.body, rec.Code, rec.Body.String())
			continue
		}
		if c.code != "" && errBody(t, rec).Code != c.code {
			t.Errorf("%s: %s", c.body, rec.Body.String())
		}
	}
	rec := h.do("POST", "/echo", `{"name":""}`, nil)
	if errBody(t, rec).Fields["name"] != "required" {
		t.Fatal(rec.Body.String())
	}
}

func TestRouter_AuthnAuthz(t *testing.T) {
	h := newHarness(t, nil)
	if rec := h.do("GET", "/admin", "", nil); rec.Code != 403 {
		t.Fatal(rec.Code)
	}
	if rec := h.do("GET", "/admin", "", map[string]string{"Authorization": "Bearer bad"}); rec.Code != 401 {
		t.Fatal(rec.Code)
	}
	if rec := h.do("GET", "/admin", "", map[string]string{"Authorization": "Bearer admin"}); rec.Code != 204 {
		t.Fatal(rec.Code)
	}
	if !strings.Contains(h.logs.String(), `"subject":"admin"`) {
		t.Fatal(h.logs.String())
	}
}

func TestRouter_RateLimitPerClassAndPrincipal(t *testing.T) {
	h := newHarness(t, nil)
	if rec := h.do("POST", "/login", "", nil); rec.Code != 204 {
		t.Fatal(rec.Code)
	}
	rec := h.do("POST", "/login", "", nil)
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "60" {
		t.Fatal(rec.Code, rec.Header())
	}
	if rec := h.do("POST", "/login", "", map[string]string{"Authorization": "Bearer other"}); rec.Code != 204 {
		t.Fatal("a different principal has its own bucket", rec.Code)
	}
	if rec := h.do("GET", "/ok", "", nil); rec.Code != 200 {
		t.Fatal("route classes are independent")
	}
}

func TestRouter_RateLimiterFailureFailsOpen(t *testing.T) {
	h := newHarness(t, errLimiter{})
	if rec := h.do("GET", "/ok", "", nil); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
}

func TestRouter_CORS(t *testing.T) {
	h := newHarness(t, nil)
	rec := h.do("OPTIONS", "/ok", "", map[string]string{"Origin": "https://app.example", "Access-Control-Request-Method": "GET"})
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Methods") == "" || rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Fatal(rec.Code, rec.Header())
	}
	rec = h.do("OPTIONS", "/ok", "", map[string]string{"Origin": "https://evil.example", "Access-Control-Request-Method": "GET"})
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Access-Control-Allow-Methods") != "" {
		t.Fatal(rec.Header())
	}
	rec = h.do("GET", "/ok", "", map[string]string{"Origin": "https://app.example"})
	if rec.Header().Get("Access-Control-Expose-Headers") == "" {
		t.Fatal(rec.Header())
	}
	wild := CORS([]string{"*"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://any.example")
	w := httptest.NewRecorder()
	wild.ServeHTTP(w, r)
	if w.Header().Get("Access-Control-Allow-Origin") != "https://any.example" {
		t.Fatal(w.Header())
	}
}

func TestRouter_StatusDefaults(t *testing.T) {
	h := newHarness(t, nil)
	if rec := h.do("GET", "/raw", "", nil); rec.Code != 200 || rec.Body.String() != "raw" {
		t.Fatal(rec.Code)
	}
	if rec := h.do("GET", "/silent", "", nil); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
}

func TestRouter_DuplicateRouteRejectedAndRoutesListed(t *testing.T) {
	h := newHarness(t, nil)
	if err := h.router.Mount(Route{Method: "GET", Path: "/ok"}); err == nil {
		t.Fatal("duplicate must fail")
	}
	if rec := h.do("POST", "/big", `{"name":"`+strings.Repeat("x", 500)+`"}`, nil); rec.Code != 200 {
		t.Fatal("per-route body limit", rec.Code, rec.Body.String())
	}
	if len(h.router.Routes()) != 9 {
		t.Fatal(len(h.router.Routes()))
	}
}

func TestStatusMapping(t *testing.T) {
	want := map[errs.Kind]int{errs.KindValidation: 400, errs.KindNotFound: 404, errs.KindConflict: 409,
		errs.KindUnauthorized: 401, errs.KindForbidden: 403, errs.KindRateLimited: 429,
		errs.KindUnavailable: 503, errs.KindInternal: 500}
	for k, s := range want {
		if Status(k) != s {
			t.Errorf("%v", k)
		}
	}
}

func TestResolveIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "garbage"
	r.Header.Set("X-Forwarded-For", "1.1.1.1")
	if resolveIP(r, false) != "garbage" {
		t.Fatal("untrusted proxy headers must be ignored")
	}
	if ClientIP(context.Background()) != "" {
		t.Fatal()
	}
	SetSubject(context.Background(), "x") // no-op without request info
}

func TestQueryHelpers(t *testing.T) {
	r := httptest.NewRequest("GET", "/?n=5&bad=x&big=500&f=1.5&fbad=zz", nil)
	if n, err := QueryInt(r, "n", 1, 0, 10); n != 5 || err != nil {
		t.Fatal(n, err)
	}
	if n, _ := QueryInt(r, "missing", 7, 0, 10); n != 7 {
		t.Fatal(n)
	}
	for _, name := range []string{"bad", "big"} {
		if _, err := QueryInt(r, name, 0, 0, 10); err == nil {
			t.Fatal(name)
		}
	}
	if f, err := QueryFloat(r, "f", 0, 2); f != 1.5 || err != nil {
		t.Fatal(f, err)
	}
	if _, err := QueryFloat(r, "fbad", 0, 2); err == nil {
		t.Fatal()
	}
	if _, err := QueryFloat(r, "f", 0, 1); err == nil {
		t.Fatal()
	}
}

func TestPathID(t *testing.T) {
	mux := http.NewServeMux()
	var got string
	var gotErr error
	mux.HandleFunc("/x/{id}", func(w http.ResponseWriter, r *http.Request) { got, gotErr = PathID(r, "id") })
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x/00000000-0000-7000-8000-000000000001", nil))
	if gotErr != nil || got == "" {
		t.Fatal(gotErr)
	}
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x/nope", nil))
	if gotErr == nil {
		t.Fatal("invalid uuid accepted")
	}
}

func TestStatusRecorder(t *testing.T) {
	w := httptest.NewRecorder()
	s := &statusRecorder{ResponseWriter: w}
	if s.Unwrap() != w {
		t.Fatal()
	}
	s.WriteHeader(201)
	s.WriteHeader(500)
	_, _ = io.WriteString(s, "x")
	if s.status != 201 || s.bytes != 1 {
		t.Fatal(s.status)
	}
}
