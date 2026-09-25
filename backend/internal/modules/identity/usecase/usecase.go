// Package usecase implements identity interactors: one struct per operation,
// each with a single Execute method. Authorization is checked here.
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/tx"
)

// Policy holds the tunable auth rules (from config).
type Policy struct {
	OTPTTL         time.Duration
	RefreshTTL     time.Duration
	OTPLength      int
	OTPMaxAttempts int
	OTPPerNumber   int
	OTPPerIP       int
	GuestMode      bool
	ExposeOTP      bool
}

// Deps are the collaborators shared by every identity use case.
type Deps struct {
	Users      domain.Users
	Challenges domain.Challenges
	Sessions   domain.Sessions
	Hasher     domain.SecretHasher
	SMS        domain.SMSSender
	Limiter    domain.Limiter
	Access     domain.AccessIssuer
	Tx         tx.Manager
	Clock      clock.Clock
	IDs        idgen.Generator
	Random     io.Reader
	Events     eventbus.Publisher
	Factory    eventbus.Factory
	Policy     Policy
}

// TokenPair is an access token plus its rotating refresh token.
type TokenPair struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// AuthResult is returned by sign-in flows.
type AuthResult struct {
	Tokens TokenPair
	User   domain.User
}

// TopicUserRegistered is published when an account gains a verified phone.
const TopicUserRegistered = "identity.user.registered"

// HashToken is the at-rest form of a refresh token (SHA-256; tokens carry
// 256 bits of entropy so a slow hash adds nothing).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomDigits(r io.Reader, n int) (string, error) {
	out := make([]byte, 0, n)
	buf := make([]byte, 1)
	for len(out) < n {
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		if buf[0] >= 250 { // rejection sampling keeps digits uniform
			continue
		}
		out = append(out, '0'+buf[0]%10)
	}
	return string(out), nil
}

func randomToken(r io.Reader) (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// openSession creates a session for u and mints its first token pair.
func (d Deps) openSession(ctx context.Context, u domain.User) (TokenPair, error) {
	now := d.Clock.Now()
	s := domain.Session{ID: d.IDs.New(), UserID: u.ID, CreatedAt: now, ExpiresAt: now.Add(d.Policy.RefreshTTL)}
	if err := d.Sessions.Create(ctx, s); err != nil {
		return TokenPair{}, err
	}
	return d.mint(ctx, u, s)
}

// mint issues an access token and a new refresh token in session s.
func (d Deps) mint(ctx context.Context, u domain.User, s domain.Session) (TokenPair, error) {
	raw, err := randomToken(d.Random)
	if err != nil {
		return TokenPair{}, err
	}
	now := d.Clock.Now()
	rt := domain.RefreshToken{Hash: HashToken(raw), SessionID: s.ID, CreatedAt: now, ExpiresAt: s.ExpiresAt}
	if err := d.Sessions.AddToken(ctx, rt); err != nil {
		return TokenPair{}, err
	}
	access, exp, err := d.Access.IssueAccess(u.ID, s.ID, u.Role)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, AccessExpiresAt: exp, RefreshToken: raw, RefreshExpiresAt: rt.ExpiresAt}, nil
}

func (d Deps) publish(ctx context.Context, topic, actor string, payload any) error {
	e, err := d.Factory.New(ctx, topic, actor, payload)
	if err == nil {
		err = d.Events.Publish(ctx, e)
	}
	return err
}
