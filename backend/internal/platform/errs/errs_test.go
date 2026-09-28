package errs

import (
	"errors"
	"fmt"
	"testing"
)

func TestKind_String(t *testing.T) {
	want := map[Kind]string{KindInternal: "internal", KindValidation: "validation", KindNotFound: "not_found",
		KindConflict: "conflict", KindUnauthorized: "unauthorized", KindForbidden: "forbidden",
		KindRateLimited: "rate_limited", KindUnavailable: "unavailable", Kind(99): "unknown", Kind(-1): "unknown"}
	for k, s := range want {
		if k.String() != s {
			t.Errorf("%d: %s", k, k.String())
		}
	}
}

func TestConstructors_SetKindAndMessageKey(t *testing.T) {
	cases := []struct {
		e    *Error
		kind Kind
	}{
		{Validation("a.b"), KindValidation}, {NotFound("a.b"), KindNotFound}, {Conflict("a.b"), KindConflict},
		{Unauthorized("a.b"), KindUnauthorized}, {Forbidden("a.b"), KindForbidden},
		{RateLimited("a.b"), KindRateLimited}, {Unavailable("a.b"), KindUnavailable}, {Internal("a.b"), KindInternal},
	}
	for _, c := range cases {
		if c.e.Kind != c.kind || c.e.Message != "errors.a.b" || c.e.Error() != "a.b" {
			t.Errorf("%+v", c.e)
		}
	}
}

func TestWrapAndFields_CopyAndMatchSentinel(t *testing.T) {
	sentinel := NotFound("scan.not_found")
	cause := errors.New("db down")
	w := sentinel.Wrap(cause).WithField("id", "x").WithField("n", "y")
	if sentinel.Fields != nil || sentinel.Unwrap() != nil {
		t.Fatal("sentinel mutated")
	}
	if !errors.Is(w, sentinel) || !errors.Is(w, cause) || w.Error() != "scan.not_found: db down" {
		t.Fatalf("unexpected %v", w)
	}
	if errors.Is(w, NotFound("other")) || w.Is(cause) {
		t.Fatal("must not match different code")
	}
	if len(w.Fields) != 2 {
		t.Fatal(w.Fields)
	}
}

func TestAsAndFrom(t *testing.T) {
	if From(nil) != nil {
		t.Fatal("nil")
	}
	e := Conflict("x.y")
	if From(fmt.Errorf("ctx: %w", e)) != e {
		t.Fatal("must unwrap")
	}
	internal := From(errors.New("raw"))
	if internal.Kind != KindInternal || internal.Code != "internal" {
		t.Fatal(internal)
	}
	if _, ok := As(errors.New("raw")); ok {
		t.Fatal("As on plain error")
	}
}
