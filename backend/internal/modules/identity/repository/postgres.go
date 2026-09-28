// Package repository implements the identity ports on Postgres.
package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
)

// Users implements domain.Users.
type Users struct{ DB *database.DB }

const userCols = `id, COALESCE(phone, ''), name, language, district, role, created_at, updated_at`

func scanUser(row pgx.Row) (domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.Phone, &u.Name, &u.Language, &u.District, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	u.CreatedAt, u.UpdatedAt = u.CreatedAt.UTC(), u.UpdatedAt.UTC()
	return u, database.NotFound(err, domain.ErrUserNotFound)
}

// Create inserts u; a duplicate phone is a conflict.
func (r Users) Create(ctx context.Context, u domain.User) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO identity_users (id, phone, name, language, district, role, created_at, updated_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8)`, u.ID, u.Phone, u.Name, u.Language, u.District, u.Role, u.CreatedAt, u.UpdatedAt)
	return phoneConflict(err)
}

func phoneConflict(err error) error {
	if database.IsUniqueViolation(err) {
		return domain.ErrPhoneTaken
	}
	return err
}

// Get loads a user by ID.
func (r Users) Get(ctx context.Context, id string) (domain.User, error) {
	return scanUser(r.DB.Q(ctx).QueryRow(ctx, `SELECT `+userCols+` FROM identity_users WHERE id = $1`, id))
}

// GetByPhone loads a user by E.164 phone.
func (r Users) GetByPhone(ctx context.Context, phone string) (domain.User, error) {
	return scanUser(r.DB.Q(ctx).QueryRow(ctx, `SELECT `+userCols+` FROM identity_users WHERE phone = $1`, phone))
}

// Update persists mutable profile fields.
func (r Users) Update(ctx context.Context, u domain.User) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `UPDATE identity_users SET phone = NULLIF($2, ''), name = $3, language = $4,
		district = $5, role = $6, updated_at = $7 WHERE id = $1`, u.ID, u.Phone, u.Name, u.Language, u.District, u.Role, u.UpdatedAt)
	return phoneConflict(err)
}

// Challenges implements domain.Challenges.
type Challenges struct{ DB *database.DB }

// Create stores a challenge.
func (r Challenges) Create(ctx context.Context, c domain.Challenge) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO identity_otp_challenges (id, phone, code_hash, attempts, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, c.ID, c.Phone, c.CodeHash, c.Attempts, c.ExpiresAt, c.CreatedAt)
	return err
}

// Latest returns the newest unconsumed challenge for phone, locking it.
func (r Challenges) Latest(ctx context.Context, phone string) (domain.Challenge, error) {
	var c domain.Challenge
	err := r.DB.Q(ctx).QueryRow(ctx, `SELECT id, phone, code_hash, attempts, expires_at, created_at
		FROM identity_otp_challenges WHERE phone = $1 AND consumed_at IS NULL
		ORDER BY created_at DESC, id DESC LIMIT 1 FOR UPDATE`, phone).
		Scan(&c.ID, &c.Phone, &c.CodeHash, &c.Attempts, &c.ExpiresAt, &c.CreatedAt)
	return c, database.NotFound(err, domain.ErrOTPInvalid)
}

// IncrementAttempts records a failed verification.
func (r Challenges) IncrementAttempts(ctx context.Context, id string) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `UPDATE identity_otp_challenges SET attempts = attempts + 1 WHERE id = $1`, id)
	return err
}

// Consume marks the challenge used.
func (r Challenges) Consume(ctx context.Context, id string, at time.Time) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `UPDATE identity_otp_challenges SET consumed_at = $2 WHERE id = $1`, id, at)
	return err
}

// Sessions implements domain.Sessions.
type Sessions struct{ DB *database.DB }

// Create stores a session.
func (r Sessions) Create(ctx context.Context, s domain.Session) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO identity_sessions (id, user_id, created_at, expires_at) VALUES ($1, $2, $3, $4)`,
		s.ID, s.UserID, s.CreatedAt, s.ExpiresAt)
	return err
}

// Get loads a session.
func (r Sessions) Get(ctx context.Context, id string) (domain.Session, error) {
	var s domain.Session
	err := r.DB.Q(ctx).QueryRow(ctx, `SELECT id, user_id, created_at, expires_at, revoked_at FROM identity_sessions WHERE id = $1`, id).
		Scan(&s.ID, &s.UserID, &s.CreatedAt, &s.ExpiresAt, &s.RevokedAt)
	return s, database.NotFound(err, domain.ErrSessionRevoked)
}

// Revoke ends a session (idempotent; the first revocation time is kept).
func (r Sessions) Revoke(ctx context.Context, id string, at time.Time) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `UPDATE identity_sessions SET revoked_at = COALESCE(revoked_at, $2) WHERE id = $1`, id, at)
	return err
}

// AddToken stores a hashed refresh token.
func (r Sessions) AddToken(ctx context.Context, t domain.RefreshToken) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO identity_refresh_tokens (token_hash, session_id, created_at, expires_at) VALUES ($1, $2, $3, $4)`,
		t.Hash, t.SessionID, t.CreatedAt, t.ExpiresAt)
	return err
}

// GetToken loads a refresh token by hash.
func (r Sessions) GetToken(ctx context.Context, hash string) (domain.RefreshToken, error) {
	var t domain.RefreshToken
	err := r.DB.Q(ctx).QueryRow(ctx, `SELECT token_hash, session_id, created_at, expires_at, used_at FROM identity_refresh_tokens WHERE token_hash = $1`, hash).
		Scan(&t.Hash, &t.SessionID, &t.CreatedAt, &t.ExpiresAt, &t.UsedAt)
	return t, database.NotFound(err, domain.ErrRefreshInvalid)
}

// MarkUsed atomically flips used_at; false means the token was already used.
func (r Sessions) MarkUsed(ctx context.Context, hash string, at time.Time) (bool, error) {
	tag, err := r.DB.Q(ctx).Exec(ctx, `UPDATE identity_refresh_tokens SET used_at = $2 WHERE token_hash = $1 AND used_at IS NULL`, hash, at)
	return tag.RowsAffected() == 1, err
}
