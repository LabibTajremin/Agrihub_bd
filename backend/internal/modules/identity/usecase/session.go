package usecase

import (
	"context"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
)

// Refresh rotates a refresh token. Presenting an already-used token is a
// replay: the whole session (token family) is revoked.
type Refresh struct{ Deps }

// Execute rotates token.
func (uc Refresh) Execute(ctx context.Context, token string) (TokenPair, error) {
	hash := HashToken(token)
	var pair TokenPair
	reused := false
	err := uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		rt, err := uc.Sessions.GetToken(ctx, hash)
		if err != nil {
			return err
		}
		now := uc.Clock.Now()
		if !now.Before(rt.ExpiresAt) {
			return domain.ErrRefreshInvalid
		}
		s, err := uc.Sessions.Get(ctx, rt.SessionID)
		if err != nil {
			return err
		}
		if !s.Active(now) {
			return domain.ErrSessionRevoked
		}
		fresh, err := uc.Sessions.MarkUsed(ctx, hash, now)
		if err != nil {
			return err
		}
		if !fresh {
			reused = true
			return uc.Sessions.Revoke(ctx, s.ID, now)
		}
		u, err := uc.Users.Get(ctx, s.UserID)
		if err != nil {
			return err
		}
		pair, err = uc.mint(ctx, u, s)
		return err
	})
	if err != nil {
		return TokenPair{}, err
	}
	if reused {
		return TokenPair{}, domain.ErrRefreshReused
	}
	return pair, nil
}

// Logout revokes the caller's session.
type Logout struct{ Deps }

// Execute revokes the session in the access token.
func (uc Logout) Execute(ctx context.Context) error {
	p, err := authn.Require(ctx, authz.ProfileRead)
	if err != nil {
		return err
	}
	return uc.Sessions.Revoke(ctx, p.SessionID, uc.Clock.Now())
}

// GetMe returns the caller's profile.
type GetMe struct{ Deps }

// Execute loads the caller.
func (uc GetMe) Execute(ctx context.Context) (domain.User, error) {
	p, err := authn.Require(ctx, authz.ProfileRead)
	if err != nil {
		return domain.User{}, err
	}
	return uc.Users.Get(ctx, p.Subject)
}

// UpdateMeInput carries optional profile changes.
type UpdateMeInput struct {
	Name     *string
	Language *string
	District *string
}

// UpdateMe edits the caller's profile.
type UpdateMe struct{ Deps }

// Execute applies the provided fields.
func (uc UpdateMe) Execute(ctx context.Context, in UpdateMeInput) (domain.User, error) {
	p, err := authn.Require(ctx, authz.ProfileWrite)
	if err != nil {
		return domain.User{}, err
	}
	if in.Language != nil && !domain.ValidLanguage(*in.Language) {
		return domain.User{}, domain.ErrInvalidProfile.WithField("language", "unsupported")
	}
	u, err := uc.Users.Get(ctx, p.Subject)
	if err != nil {
		return domain.User{}, err
	}
	if in.Name != nil {
		u.Name = *in.Name
	}
	if in.Language != nil {
		u.Language = *in.Language
	}
	if in.District != nil {
		u.District = *in.District
	}
	u.UpdatedAt = uc.Clock.Now()
	return u, uc.Users.Update(ctx, u)
}
