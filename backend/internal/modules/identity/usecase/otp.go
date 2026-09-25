package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
)

// RequestOTP sends a one-time code to a phone number.
type RequestOTP struct{ Deps }

// RequestOTPInput is the request.
type RequestOTPInput struct {
	Phone    string
	ClientIP string
}

// RequestOTPResult reports when the code expires. DevCode is set only when
// features.expose_otp is enabled (never in production).
type RequestOTPResult struct {
	ExpiresAt time.Time
	DevCode   string
}

// Execute applies per-IP and per-number limits, stores the hashed code and sends it.
func (uc RequestOTP) Execute(ctx context.Context, in RequestOTPInput) (RequestOTPResult, error) {
	phone, err := domain.NormalizePhone(in.Phone)
	if err != nil {
		return RequestOTPResult{}, err
	}
	for _, lim := range []struct {
		key string
		n   int
	}{{"otp:ip:" + in.ClientIP, uc.Policy.OTPPerIP}, {"otp:phone:" + phone, uc.Policy.OTPPerNumber}} {
		ok, err := uc.Limiter.AllowPerHour(ctx, lim.key, lim.n)
		if err != nil {
			return RequestOTPResult{}, err
		}
		if !ok {
			return RequestOTPResult{}, domain.ErrOTPRateLimited
		}
	}
	code, err := randomDigits(uc.Random, uc.Policy.OTPLength)
	if err != nil {
		return RequestOTPResult{}, err
	}
	hash, err := uc.Hasher.Hash(code)
	if err != nil {
		return RequestOTPResult{}, err
	}
	now := uc.Clock.Now()
	ch := domain.Challenge{ID: uc.IDs.New(), Phone: phone, CodeHash: hash, ExpiresAt: now.Add(uc.Policy.OTPTTL), CreatedAt: now}
	if err := uc.Challenges.Create(ctx, ch); err != nil {
		return RequestOTPResult{}, err
	}
	if err := uc.SMS.SendOTP(ctx, phone, code); err != nil {
		return RequestOTPResult{}, err
	}
	res := RequestOTPResult{ExpiresAt: ch.ExpiresAt}
	if uc.Policy.ExposeOTP {
		res.DevCode = code
	}
	return res, nil
}

// VerifyOTP exchanges a valid code for a session. A guest caller is upgraded
// in place so its scans keep their owner.
type VerifyOTP struct{ Deps }

// VerifyOTPInput is the request.
type VerifyOTPInput struct {
	Phone string
	Code  string
}

// Execute verifies the latest challenge. Failed attempts are committed even
// though the call fails, so the attempt ceiling cannot be bypassed.
func (uc VerifyOTP) Execute(ctx context.Context, in VerifyOTPInput) (AuthResult, error) {
	phone, err := domain.NormalizePhone(in.Phone)
	if err != nil {
		return AuthResult{}, err
	}
	caller := authn.FromContext(ctx)
	var res AuthResult
	var failure error
	err = uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		ch, err := uc.Challenges.Latest(ctx, phone)
		if err != nil {
			return err
		}
		now := uc.Clock.Now()
		switch {
		case !now.Before(ch.ExpiresAt):
			return domain.ErrOTPExpired
		case ch.Attempts >= uc.Policy.OTPMaxAttempts:
			return domain.ErrOTPLocked
		case !uc.Hasher.Verify(in.Code, ch.CodeHash):
			failure = domain.ErrOTPInvalid
			return uc.Challenges.IncrementAttempts(ctx, ch.ID)
		}
		if err := uc.Challenges.Consume(ctx, ch.ID, now); err != nil {
			return err
		}
		user, err := uc.resolveUser(ctx, phone, caller)
		if err != nil {
			return err
		}
		tokens, err := uc.openSession(ctx, user)
		res = AuthResult{Tokens: tokens, User: user}
		return err
	})
	if err != nil {
		return AuthResult{}, err
	}
	if failure != nil {
		return AuthResult{}, failure
	}
	return res, nil
}

func (uc VerifyOTP) resolveUser(ctx context.Context, phone string, caller authn.Principal) (domain.User, error) {
	u, err := uc.Users.GetByPhone(ctx, phone)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, domain.ErrUserNotFound) {
		return domain.User{}, err
	}
	now := uc.Clock.Now()
	if !caller.Anonymous && caller.Role == authz.Guest {
		u, err = uc.Users.Get(ctx, caller.Subject)
		if err != nil {
			return domain.User{}, err
		}
		u.Phone, u.Role, u.UpdatedAt = phone, domain.RoleFarmer, now
		if err := uc.Users.Update(ctx, u); err != nil {
			return domain.User{}, err
		}
	} else {
		u = domain.User{ID: uc.IDs.New(), Phone: phone, Language: domain.Languages[0], Role: domain.RoleFarmer, CreatedAt: now, UpdatedAt: now}
		if err := uc.Users.Create(ctx, u); err != nil {
			return domain.User{}, err
		}
	}
	return u, uc.publish(ctx, TopicUserRegistered, u.ID, map[string]string{"user_id": u.ID})
}

// StartGuest creates an anonymous-but-identified guest account so a first
// scan works without sign-up.
type StartGuest struct{ Deps }

// Execute creates the guest and its session.
func (uc StartGuest) Execute(ctx context.Context) (AuthResult, error) {
	if !uc.Policy.GuestMode {
		return AuthResult{}, domain.ErrGuestDisabled
	}
	var res AuthResult
	err := uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		now := uc.Clock.Now()
		u := domain.User{ID: uc.IDs.New(), Language: domain.Languages[0], Role: domain.RoleGuest, CreatedAt: now, UpdatedAt: now}
		if err := uc.Users.Create(ctx, u); err != nil {
			return err
		}
		tokens, err := uc.openSession(ctx, u)
		res = AuthResult{Tokens: tokens, User: u}
		return err
	})
	return res, err
}
