//go:build integration

package apitest

import (
	"net/http"
	"testing"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/test/harness"
)

type fakeTB struct {
	testing.TB
	failed int
}

func (f *fakeTB) Helper()               {}
func (f *fakeTB) Fatal(...any)          { f.failed++ }
func (f *fakeTB) Fatalf(string, ...any) { f.failed++ }

func TestAPI_RoundTrip(t *testing.T) {
	a := New(t, harness.DB(t))
	a.Mount([]httpx.Route{{Method: http.MethodPost, Path: "/echo", Permission: string(authz.ProfileRead),
		Handler: func(w http.ResponseWriter, r *http.Request) error {
			return httpx.JSON(w, http.StatusOK, map[string]string{"h": r.Header.Get("X-Test")})
		}}})
	res := a.Do("POST", "/echo", map[string]int{"a": 1}, a.Token("u", authz.Farmer), "X-Test", "yes").Expect(t, 200)
	var out map[string]string
	res.Decode(t, &out)
	if out["h"] != "yes" {
		t.Fatal(out)
	}
	if code := a.Do("POST", "/echo", "raw", "").ErrorCode(); code != "auth.required" {
		t.Fatal(code)
	}
	a.Do("POST", "/echo", nil, "").Expect(t, 401)

	f := &fakeTB{TB: t}
	b := New(f, harness.DB(t))
	b.Mount([]httpx.Route{{Method: "GET", Path: "/x"}, {Method: "GET", Path: "/x"}})
	b.Do("GET", "/x", make(chan int), "")
	(&Response{Body: []byte("{")}).Decode(f, &out)
	(&Response{Code: 500}).Expect(f, 200)
	if f.failed != 4 {
		t.Fatal(f.failed)
	}
}
