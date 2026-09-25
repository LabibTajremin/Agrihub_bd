// Package transport exposes identity over HTTP: DTOs, validation, handlers.
package transport

import (
	"net/http"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// PhoneRequest asks for an OTP.
type PhoneRequest struct {
	Phone string `json:"phone" validate:"required,max=24"`
}

// OTPRequested reports the code expiry (and the code itself in development).
type OTPRequested struct {
	ExpiresAt time.Time `json:"expires_at"`
	DevCode   string    `json:"dev_code,omitempty"`
}

// VerifyRequest submits a code.
type VerifyRequest struct {
	Phone string `json:"phone" validate:"required,max=24"`
	Code  string `json:"code" validate:"required,numeric,min=4,max=10"`
}

// RefreshRequest rotates a refresh token.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required,max=256"`
}

// Tokens is an access + refresh token pair.
type Tokens struct {
	TokenType        string    `json:"token_type"`
	AccessToken      string    `json:"access_token"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

// User is the public profile.
type User struct {
	ID        string    `json:"id"`
	Phone     string    `json:"phone,omitempty"`
	Name      string    `json:"name"`
	Language  string    `json:"language"`
	District  string    `json:"district"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// Session is returned by sign-in flows.
type Session struct {
	Tokens
	User User `json:"user"`
}

// UpdateMeRequest carries optional profile fields.
type UpdateMeRequest struct {
	Name     *string `json:"name" validate:"omitempty,max=80"`
	Language *string `json:"language" validate:"omitempty,oneof=bn en hi es fr ar pt"`
	District *string `json:"district" validate:"omitempty,max=60"`
}

// Handlers groups the identity endpoints.
type Handlers struct {
	RequestOTP usecase.RequestOTP
	VerifyOTP  usecase.VerifyOTP
	StartGuest usecase.StartGuest
	Refresh    usecase.Refresh
	Logout     usecase.Logout
	GetMe      usecase.GetMe
	UpdateMe   usecase.UpdateMe
	V          *validator.Validator
}

func toTokens(p usecase.TokenPair) Tokens {
	return Tokens{TokenType: "Bearer", AccessToken: p.AccessToken, ExpiresAt: p.AccessExpiresAt,
		RefreshToken: p.RefreshToken, RefreshExpiresAt: p.RefreshExpiresAt}
}

func toUser(u domain.User) User {
	return User{ID: u.ID, Phone: u.Phone, Name: u.Name, Language: u.Language, District: u.District, Role: u.Role, CreatedAt: u.CreatedAt}
}

// Routes declares the endpoints.
func (h Handlers) Routes() []httpx.Route {
	const tag = "auth"
	return []httpx.Route{
		{Method: http.MethodPost, Path: "/v1/auth/otp/request", Class: httpx.ClassAuth, Tag: tag, Summary: "Send a one-time code by SMS",
			Request: PhoneRequest{}, Response: OTPRequested{}, Status: http.StatusAccepted, Handler: h.requestOTP},
		{Method: http.MethodPost, Path: "/v1/auth/otp/verify", Class: httpx.ClassAuth, Tag: tag, Summary: "Exchange a code for a session",
			Request: VerifyRequest{}, Response: Session{}, Handler: h.verifyOTP},
		{Method: http.MethodPost, Path: "/v1/auth/guest", Class: httpx.ClassAuth, Tag: tag, Summary: "Start a guest session",
			Response: Session{}, Status: http.StatusCreated, Handler: h.guest},
		{Method: http.MethodPost, Path: "/v1/auth/refresh", Class: httpx.ClassAuth, Tag: tag, Summary: "Rotate a refresh token",
			Request: RefreshRequest{}, Response: Tokens{}, Handler: h.refresh},
		{Method: http.MethodPost, Path: "/v1/auth/logout", Permission: string(authz.ProfileRead), Tag: tag, Summary: "Revoke the current session",
			Status: http.StatusNoContent, Handler: h.logout},
		{Method: http.MethodGet, Path: "/v1/me", Permission: string(authz.ProfileRead), Tag: "profile", Summary: "Current profile",
			Response: User{}, Handler: h.me},
		{Method: http.MethodPatch, Path: "/v1/me", Permission: string(authz.ProfileWrite), Tag: "profile", Summary: "Update profile",
			Request: UpdateMeRequest{}, Response: User{}, Handler: h.updateMe},
	}
}

func (h Handlers) requestOTP(w http.ResponseWriter, r *http.Request) error {
	var in PhoneRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	res, err := h.RequestOTP.Execute(r.Context(), usecase.RequestOTPInput{Phone: in.Phone, ClientIP: httpx.ClientIP(r.Context())})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusAccepted, OTPRequested{ExpiresAt: res.ExpiresAt, DevCode: res.DevCode})
}

func (h Handlers) verifyOTP(w http.ResponseWriter, r *http.Request) error {
	var in VerifyRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	res, err := h.VerifyOTP.Execute(r.Context(), usecase.VerifyOTPInput{Phone: in.Phone, Code: in.Code})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, Session{Tokens: toTokens(res.Tokens), User: toUser(res.User)})
}

func (h Handlers) guest(w http.ResponseWriter, r *http.Request) error {
	res, err := h.StartGuest.Execute(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, Session{Tokens: toTokens(res.Tokens), User: toUser(res.User)})
}

func (h Handlers) refresh(w http.ResponseWriter, r *http.Request) error {
	var in RefreshRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	pair, err := h.Refresh.Execute(r.Context(), in.RefreshToken)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, toTokens(pair))
}

func (h Handlers) logout(w http.ResponseWriter, r *http.Request) error {
	if err := h.Logout.Execute(r.Context()); err != nil {
		return err
	}
	return httpx.NoContent(w)
}

func (h Handlers) me(w http.ResponseWriter, r *http.Request) error {
	u, err := h.GetMe.Execute(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, toUser(u))
}

func (h Handlers) updateMe(w http.ResponseWriter, r *http.Request) error {
	var in UpdateMeRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	u, err := h.UpdateMe.Execute(r.Context(), usecase.UpdateMeInput{Name: in.Name, Language: in.Language, District: in.District})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, toUser(u))
}
