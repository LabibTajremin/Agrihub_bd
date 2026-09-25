package authn

import (
	"slices"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
)

// Claims is the access-token payload: sub, sid, role, perms, iat, exp, jti.
type Claims struct {
	jwt.RegisteredClaims
	SessionID   string   `json:"sid"`
	Role        string   `json:"role"`
	Permissions []string `json:"perms"`
}

// SignerOptions configures token signing.
type SignerOptions struct {
	// Keys maps kid → HMAC secret; ActiveKID signs, every key verifies.
	Keys      map[string][]byte
	ActiveKID string
	Issuer    string
	TTL       time.Duration
	Clock     clock.Clock
	IDs       idgen.Generator
}

// Signer issues and verifies access tokens.
type Signer struct{ o SignerOptions }

// NewSigner returns a Signer.
func NewSigner(o SignerOptions) *Signer { return &Signer{o: o} }

// Issue signs an access token for p and returns it with its expiry.
func (s *Signer) Issue(p Principal) (string, time.Time, error) {
	now := s.o.Clock.Now()
	exp := now.Add(s.o.TTL)
	perms := authz.PermissionsOf(p.Role)
	ps := make([]string, len(perms))
	for i, x := range perms {
		ps[i] = string(x)
	}
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: p.Subject, Issuer: s.o.Issuer, ID: s.o.IDs.New(),
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp),
		},
		SessionID: p.SessionID, Role: string(p.Role), Permissions: ps,
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	t.Header["kid"] = s.o.ActiveKID
	signed, err := t.SignedString(s.o.Keys[s.o.ActiveKID])
	return signed, exp, err
}

// Verify validates signature (by kid), algorithm, issuer and expiry.
func (s *Signer) Verify(token string) (Principal, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		key, ok := s.o.Keys[kid]
		if !ok {
			return nil, ErrInvalidToken
		}
		return key, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(s.o.Issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(s.o.Clock.Now),
	)
	role := authz.Role(c.Role)
	if err != nil || c.Subject == "" || !authz.ValidRole(role) {
		return Principal{}, ErrInvalidToken
	}
	return Principal{Subject: c.Subject, SessionID: c.SessionID, Role: role}, nil
}

// HasPermission reports whether perms (from claims) contains p.
func HasPermission(perms []string, p authz.Permission) bool {
	return slices.Contains(perms, string(p))
}
