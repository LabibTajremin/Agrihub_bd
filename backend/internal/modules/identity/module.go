// Package identity is the identity module (future auth-service): users, OTP
// sign-in, guest sessions, rotating refresh tokens and profiles.
package identity

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/kernel"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/logger"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/ratelimit"
)

// Deps are the module's dependencies.
type Deps struct {
	Kernel  kernel.Kernel
	Signer  *authn.Signer
	Hasher  domain.SecretHasher
	Limiter ratelimit.Limiter
	SMS     domain.SMSSender // nil = logging stub
	Random  io.Reader
	Policy  usecase.Policy
}

// Module is the assembled identity module.
type Module struct{ handlers transport.Handlers }

// New wires repositories, use cases and handlers.
func New(d Deps) (*Module, error) {
	k := d.Kernel
	sms := d.SMS
	if sms == nil {
		sms = LogSMS{Logger: k.Logger}
	}
	ud := usecase.Deps{
		Users: repository.Users{DB: k.DB}, Challenges: repository.Challenges{DB: k.DB}, Sessions: repository.Sessions{DB: k.DB},
		Hasher: d.Hasher, SMS: sms, Limiter: limiter{d.Limiter}, Access: issuer{d.Signer},
		Tx: k.DB, Clock: k.Clock, IDs: k.IDs, Random: d.Random, Events: k.Events, Factory: k.Factory(), Policy: d.Policy,
	}
	return &Module{handlers: transport.Handlers{
		RequestOTP: usecase.RequestOTP{Deps: ud}, VerifyOTP: usecase.VerifyOTP{Deps: ud}, StartGuest: usecase.StartGuest{Deps: ud},
		Refresh: usecase.Refresh{Deps: ud}, Logout: usecase.Logout{Deps: ud}, GetMe: usecase.GetMe{Deps: ud},
		UpdateMe: usecase.UpdateMe{Deps: ud}, V: k.Validator,
	}}, nil
}

// Routes returns the module's HTTP routes.
func (m *Module) Routes() []httpx.Route { return m.handlers.Routes() }

// Errors is the module's error catalogue.
func Errors() []*errs.Error { return domain.Errors() }

type limiter struct{ l ratelimit.Limiter }

func (a limiter) AllowPerHour(ctx context.Context, key string, n int) (bool, error) {
	d, err := a.l.Allow(ctx, key, ratelimit.PerHour(n))
	return d.Allowed, err
}

type issuer struct{ s *authn.Signer }

func (a issuer) IssueAccess(userID, sessionID, role string) (string, time.Time, error) {
	return a.s.Issue(authn.Principal{Subject: userID, SessionID: sessionID, Role: authz.Role(role)})
}

// LogSMS is the SMS stub: it logs a fingerprint of the number, never the code.
// A real provider is an adapter behind domain.SMSSender.
type LogSMS struct{ Logger *slog.Logger }

// SendOTP implements domain.SMSSender.
func (s LogSMS) SendOTP(ctx context.Context, phone, _ string) error {
	s.Logger.InfoContext(ctx, "otp issued", "phone_hash", logger.HashPII(phone))
	return nil
}
