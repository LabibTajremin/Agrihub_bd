// Package apitest drives modules through the real router and middleware chain
// for HTTP round-trip integration tests.
package apitest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/kernel"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/logger"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/metrics"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/outbox"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/ratelimit"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// Epoch is the frozen start time of every API test.
var Epoch = time.Date(2026, 3, 1, 6, 0, 0, 0, time.UTC)

// JWTSecret is the test signing key.
const JWTSecret = "test-secret-test-secret-test-secret!"

// API is a router plus the kernel and signer used to build modules under test.
type API struct {
	t      testing.TB
	Router *httpx.Router
	Kernel kernel.Kernel
	Clock  *clock.Fake
	Signer *authn.Signer
	Relay  *outbox.Relay
}

// New builds an API over db with generous rate limits.
func New(t testing.TB, db *database.DB) *API {
	t.Helper()
	c := clock.NewFake(Epoch)
	ids := idgen.UUIDv7{}
	bus := eventbus.NewLocal()
	k := kernel.Kernel{DB: db, Clock: c, IDs: ids, Validator: validator.New(), Logger: logger.Discard(),
		Events: outbox.NewPublisher(db), Bus: bus}
	signer := authn.NewSigner(authn.SignerOptions{Keys: map[string][]byte{"k1": []byte(JWTSecret)}, ActiveKID: "k1",
		Issuer: "agrismart", TTL: 15 * time.Minute, Clock: c, IDs: ids})
	h := authn.HTTP{Verifier: signer}
	router := httpx.NewRouter(httpx.Options{
		Logger: k.Logger, Clock: c, IDs: ids, Metrics: metrics.Nop{}, Limiter: ratelimit.NewMemory(c),
		Rates:          map[string]ratelimit.Rate{httpx.ClassDefault: ratelimit.PerMinute(10_000), httpx.ClassAuth: ratelimit.PerMinute(10_000)},
		RequestTimeout: time.Minute, CORSOrigins: []string{"*"}, MaxBodyBytes: 1 << 20, Authn: h, Authz: h,
	})
	return &API{t: t, Router: router, Kernel: k, Clock: c, Signer: signer, Relay: outbox.NewRelay(db, bus, c, 100)}
}

// Mount registers routes, failing the test on duplicates.
func (a *API) Mount(routes []httpx.Route) {
	a.t.Helper()
	if err := a.Router.Mount(routes...); err != nil {
		a.t.Fatal(err)
	}
}

// Token mints an access token for subject with role (no session).
func (a *API) Token(subject string, role authz.Role) string {
	tok, _, _ := a.Signer.Issue(authn.Principal{Subject: subject, SessionID: subject, Role: role})
	return tok
}

// Response is a recorded HTTP response.
type Response struct {
	Code   int
	Header http.Header
	Body   []byte
}

// Do performs a request. body may be nil, a string (sent raw) or a value (JSON).
func (a *API) Do(method, path string, body any, token string, headers ...string) *Response {
	a.t.Helper()
	var r io.Reader = http.NoBody
	switch b := body.(type) {
	case nil:
	case string:
		r = bytes.NewBufferString(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			a.t.Fatal(err)
		}
		r = bytes.NewReader(raw)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, r)
	req.RemoteAddr = "192.0.2.10:5555"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	a.Router.ServeHTTP(rec, req)
	return &Response{Code: rec.Code, Header: rec.Header(), Body: rec.Body.Bytes()}
}

// Decode unmarshals the body into v, failing the test on error.
func (r *Response) Decode(t testing.TB, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatalf("decode %d %s: %v", r.Code, r.Body, err)
	}
}

// ErrorCode returns the error code of an error response ("" if none).
func (r *Response) ErrorCode() string {
	var b httpx.ErrorBody
	_ = json.Unmarshal(r.Body, &b)
	return b.Error.Code
}

// Expect fails the test unless the status matches.
func (r *Response) Expect(t testing.TB, status int) *Response {
	t.Helper()
	if r.Code != status {
		t.Fatalf("want %d, got %d: %s", status, r.Code, r.Body)
	}
	return r
}
