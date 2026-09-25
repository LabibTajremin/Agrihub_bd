// Package authn carries the authenticated principal, issues and verifies JWT
// access tokens (HS256 with a kid header, so RS256 rotation is a key change),
// hashes secrets with argon2id and implements the HTTP authenticate/authorize
// hooks.
package authn

import (
	"context"
	"net/http"
	"strings"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
)

// Principal is the caller. Anonymous callers hold the guest role but no subject.
type Principal struct {
	Subject   string
	SessionID string
	Role      authz.Role
	Anonymous bool
}

// Anonymous is the principal of a request without credentials.
func Anonymous() Principal { return Principal{Role: authz.Guest, Anonymous: true} }

type ctxKey struct{}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext returns the principal in ctx (anonymous when absent).
func FromContext(ctx context.Context) Principal {
	if p, ok := ctx.Value(ctxKey{}).(Principal); ok {
		return p
	}
	return Anonymous()
}

// Errors.
var (
	ErrRequired     = errs.Unauthorized("auth.required")
	ErrInvalidToken = errs.Unauthorized("auth.invalid_token")
)

// Require returns the principal in ctx if it is authenticated and holds perm.
// Every use case calls this first: authorization lives at the use-case boundary.
func Require(ctx context.Context, perm authz.Permission) (Principal, error) {
	p := FromContext(ctx)
	if p.Anonymous {
		return p, ErrRequired
	}
	return p, authz.Check(p.Role, perm)
}

// HTTP implements httpx.Authenticator and httpx.Authorizer.
type HTTP struct {
	Verifier *Signer
}

var _ httpx.Authenticator = HTTP{}
var _ httpx.Authorizer = HTTP{}

// Authenticate resolves a Bearer token; no header means anonymous.
func (h HTTP) Authenticate(r *http.Request) (context.Context, error) {
	raw := r.Header.Get("Authorization")
	if raw == "" {
		return WithPrincipal(r.Context(), Anonymous()), nil
	}
	tok, ok := strings.CutPrefix(raw, "Bearer ")
	if !ok {
		return nil, ErrInvalidToken
	}
	p, err := h.Verifier.Verify(tok)
	if err != nil {
		return nil, err
	}
	httpx.SetSubject(r.Context(), p.Subject)
	return WithPrincipal(r.Context(), p), nil
}

// Authorize fast-fails requests lacking perm.
func (h HTTP) Authorize(ctx context.Context, perm string) error {
	_, err := Require(ctx, authz.Permission(perm))
	return err
}
