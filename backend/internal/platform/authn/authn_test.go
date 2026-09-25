package authn

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func signer(c clock.Clock) *Signer {
	return NewSigner(SignerOptions{
		Keys:      map[string][]byte{"k1": []byte("0123456789abcdef0123456789abcdef"), "k0": []byte("old-key-old-key-old-key-old-key!")},
		ActiveKID: "k1", Issuer: "agrismart", TTL: 15 * time.Minute, Clock: c, IDs: &idgen.Sequence{},
	})
}

func TestSigner_RoundTripAndClaims(t *testing.T) {
	c := clock.NewFake(t0)
	s := signer(c)
	tok, exp, err := s.Issue(Principal{Subject: "u1", SessionID: "s1", Role: authz.Farmer})
	if err != nil || !exp.Equal(t0.Add(15*time.Minute)) {
		t.Fatal(err)
	}
	p, err := s.Verify(tok)
	if err != nil || p.Subject != "u1" || p.SessionID != "s1" || p.Role != authz.Farmer || p.Anonymous {
		t.Fatal(p, err)
	}
	parsed, _, _ := jwt.NewParser().ParseUnverified(tok, &Claims{})
	cl := parsed.Claims.(*Claims)
	if parsed.Header["kid"] != "k1" || cl.ID == "" || cl.IssuedAt == nil || !HasPermission(cl.Permissions, authz.FieldCreate) || HasPermission(cl.Permissions, authz.UserManage) {
		t.Fatal(parsed.Header, cl)
	}
	c.Advance(16 * time.Minute)
	if _, err := s.Verify(tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatal("expired token accepted")
	}
}

func TestSigner_RejectsTampering(t *testing.T) {
	c := clock.NewFake(t0)
	s := signer(c)
	sign := func(method jwt.SigningMethod, kid string, key any, claims Claims) string {
		tk := jwt.NewWithClaims(method, claims)
		if kid != "" {
			tk.Header["kid"] = kid
		}
		out, err := tk.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	good := Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "u", Issuer: "agrismart", ExpiresAt: jwt.NewNumericDate(t0.Add(time.Hour))}, Role: "farmer"}
	noExp := good
	noExp.ExpiresAt = nil
	badRole := good
	badRole.Role = "root"
	noSub := good
	noSub.Subject = ""
	badIss := good
	badIss.Issuer = "evil"
	key := []byte("0123456789abcdef0123456789abcdef")
	cases := map[string]string{
		"garbage":     "not-a-jwt",
		"unknown kid": sign(jwt.SigningMethodHS256, "k9", key, good),
		"no kid":      sign(jwt.SigningMethodHS256, "", key, good),
		"wrong key":   sign(jwt.SigningMethodHS256, "k1", []byte("wrong-key-wrong-key-wrong-key-!!"), good),
		"alg none":    sign(jwt.SigningMethodNone, "k1", jwt.UnsafeAllowNoneSignatureType, good),
		"hs512":       sign(jwt.SigningMethodHS512, "k1", key, good),
		"no exp":      sign(jwt.SigningMethodHS256, "k1", key, noExp),
		"bad role":    sign(jwt.SigningMethodHS256, "k1", key, badRole),
		"no subject":  sign(jwt.SigningMethodHS256, "k1", key, noSub),
		"bad issuer":  sign(jwt.SigningMethodHS256, "k1", key, badIss),
	}
	for name, tok := range cases {
		if _, err := s.Verify(tok); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
	rotated := sign(jwt.SigningMethodHS256, "k0", []byte("old-key-old-key-old-key-old-key!"), good)
	if _, err := s.Verify(rotated); err != nil {
		t.Fatal("tokens signed with a retired-but-listed kid still verify", err)
	}
}

func TestRequireAndContext(t *testing.T) {
	ctx := context.Background()
	if p := FromContext(ctx); !p.Anonymous || p.Role != authz.Guest {
		t.Fatal(p)
	}
	if _, err := Require(ctx, authz.ScanCreate); !errors.Is(err, ErrRequired) {
		t.Fatal(err)
	}
	ctx = WithPrincipal(ctx, Principal{Subject: "u", Role: authz.Guest})
	if _, err := Require(ctx, authz.FieldCreate); !errors.Is(err, authz.ErrForbidden) {
		t.Fatal(err)
	}
	if p, err := Require(ctx, authz.ScanCreate); err != nil || p.Subject != "u" {
		t.Fatal(err)
	}
}

func TestHTTP_AuthenticateAndAuthorize(t *testing.T) {
	c := clock.NewFake(t0)
	s := signer(c)
	h := HTTP{Verifier: s}
	tok, _, _ := s.Issue(Principal{Subject: "u1", Role: authz.Admin})

	r := httptest.NewRequest("GET", "/", nil)
	ctx, err := h.Authenticate(r)
	if err != nil || !FromContext(ctx).Anonymous {
		t.Fatal(err)
	}
	if err := h.Authorize(ctx, string(authz.CropRead)); !errors.Is(err, ErrRequired) {
		t.Fatal("anonymous callers must authenticate for permissioned routes", err)
	}
	r.Header.Set("Authorization", "Basic abc")
	if _, err := h.Authenticate(r); !errors.Is(err, ErrInvalidToken) {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer nope")
	if _, err := h.Authenticate(r); !errors.Is(err, ErrInvalidToken) {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+tok)
	ctx, err = h.Authenticate(r)
	if err != nil || FromContext(ctx).Subject != "u1" {
		t.Fatal(err)
	}
	if err := h.Authorize(ctx, string(authz.UserManage)); err != nil {
		t.Fatal(err)
	}
}

func TestArgon2_HashVerify(t *testing.T) {
	a := NewArgon2(Argon2Params{Time: 1, Memory: 64, Threads: 1, KeyLen: 16}, bytes.NewReader(bytes.Repeat([]byte{7}, 64)))
	h, err := a.Hash("123456")
	if err != nil || !strings.HasPrefix(h, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Fatal(h, err)
	}
	if !a.Verify("123456", h) || a.Verify("654321", h) {
		t.Fatal("verify")
	}
	parts := strings.Split(h, "$")
	bad := []string{
		"plain",
		strings.Replace(h, "argon2id", "argon2i", 1),
		"$argon2id$v=19$m=x$" + parts[4] + "$" + parts[5],
		"$argon2id$v=19$" + parts[3] + "$!!$" + parts[5],
		"$argon2id$v=19$" + parts[3] + "$" + parts[4] + "$!!",
		"$argon2id$v=19$" + parts[3] + "$" + parts[4] + "$",
	}
	for _, b := range bad {
		if a.Verify("123456", b) {
			t.Errorf("accepted %q", b)
		}
	}
	if _, err := NewArgon2(Argon2Params{}, iotest.ErrReader(errors.New("no entropy"))).Hash("x"); err == nil {
		t.Fatal("rand failure must surface")
	}
}
