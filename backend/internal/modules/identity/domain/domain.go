// Package domain holds identity entities, value objects, errors and ports.
package domain

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// Roles as stored (mirrors platform/authz; domain stays dependency-free).
const (
	RoleGuest  = "guest"
	RoleFarmer = "farmer"
)

// Languages supported for a profile.
var Languages = []string{"bn", "en", "hi", "es", "fr", "ar", "pt"}

// User is an account. Guests have no phone.
type User struct {
	ID        string
	Phone     string
	Name      string
	Language  string
	District  string
	Role      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Challenge is a pending OTP verification.
type Challenge struct {
	ID         string
	Phone      string
	CodeHash   string
	Attempts   int
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// Session is one refresh-token family.
type Session struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// Active reports whether the session can still mint tokens at now.
func (s Session) Active(now time.Time) bool { return s.RevokedAt == nil && now.Before(s.ExpiresAt) }

// RefreshToken is one member of a session's rotation chain (stored hashed).
type RefreshToken struct {
	Hash      string
	SessionID string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// Errors.
var (
	ErrInvalidPhone   = errs.Validation("auth.invalid_phone")
	ErrOTPInvalid     = errs.Unauthorized("auth.otp_invalid")
	ErrOTPExpired     = errs.Unauthorized("auth.otp_expired")
	ErrOTPLocked      = errs.RateLimited("auth.otp_locked")
	ErrOTPRateLimited = errs.RateLimited("auth.otp_rate_limited")
	ErrRefreshInvalid = errs.Unauthorized("auth.refresh_invalid")
	ErrRefreshReused  = errs.Unauthorized("auth.refresh_reused")
	ErrSessionRevoked = errs.Unauthorized("auth.session_revoked")
	ErrGuestDisabled  = errs.Forbidden("auth.guest_disabled")
	ErrUserNotFound   = errs.NotFound("user.not_found")
	ErrPhoneTaken     = errs.Conflict("user.phone_taken")
	ErrInvalidProfile = errs.Validation("user.invalid_profile")
)

// Errors lists every identity error (error catalogue).
func Errors() []*errs.Error {
	return []*errs.Error{ErrInvalidPhone, ErrOTPInvalid, ErrOTPExpired, ErrOTPLocked, ErrOTPRateLimited,
		ErrRefreshInvalid, ErrRefreshReused, ErrSessionRevoked, ErrGuestDisabled, ErrUserNotFound,
		ErrPhoneTaken, ErrInvalidProfile}
}

var (
	e164      = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
	bdMobile  = regexp.MustCompile(`^01[3-9][0-9]{8}$`)
	separator = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "")
)

// NormalizePhone returns the E.164 form. Bangladeshi local numbers
// (01XXXXXXXXX, 8801XXXXXXXXX) are expanded to +8801XXXXXXXXX.
func NormalizePhone(raw string) (string, error) {
	s := separator.Replace(strings.TrimSpace(raw))
	switch {
	case bdMobile.MatchString(s):
		s = "+88" + s
	case strings.HasPrefix(s, "880") && bdMobile.MatchString(s[2:]):
		s = "+" + s
	}
	if !e164.MatchString(s) {
		return "", ErrInvalidPhone
	}
	return s, nil
}

// ValidLanguage reports whether code is a supported language.
func ValidLanguage(code string) bool {
	for _, l := range Languages {
		if l == code {
			return true
		}
	}
	return false
}

// Users is the user repository port.
type Users interface {
	Create(ctx context.Context, u User) error
	Get(ctx context.Context, id string) (User, error)
	GetByPhone(ctx context.Context, phone string) (User, error)
	Update(ctx context.Context, u User) error
}

// Challenges is the OTP challenge repository port.
type Challenges interface {
	Create(ctx context.Context, c Challenge) error
	Latest(ctx context.Context, phone string) (Challenge, error)
	IncrementAttempts(ctx context.Context, id string) error
	Consume(ctx context.Context, id string, at time.Time) error
}

// Sessions is the session + refresh token repository port.
type Sessions interface {
	Create(ctx context.Context, s Session) error
	Get(ctx context.Context, id string) (Session, error)
	Revoke(ctx context.Context, id string, at time.Time) error
	AddToken(ctx context.Context, t RefreshToken) error
	GetToken(ctx context.Context, hash string) (RefreshToken, error)
	// MarkUsed atomically marks an unused token as used; false means it was
	// already used (a replay).
	MarkUsed(ctx context.Context, hash string, at time.Time) (bool, error)
}

// SecretHasher hashes OTP codes (argon2id adapter).
type SecretHasher interface {
	Hash(secret string) (string, error)
	Verify(secret, encoded string) bool
}

// SMSSender delivers an OTP. MVP ships a logging stub (no provider integration).
type SMSSender interface {
	SendOTP(ctx context.Context, phone, code string) error
}

// Limiter decides whether key may act again within the hour.
type Limiter interface {
	AllowPerHour(ctx context.Context, key string, limit int) (bool, error)
}

// AccessIssuer mints short-lived access tokens.
type AccessIssuer interface {
	IssueAccess(userID, sessionID, role string) (string, time.Time, error)
}
