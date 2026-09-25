// Package repository implements the diagnosis ports on Postgres.
package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
)

// Scans implements domain.Scans.
type Scans struct{ DB *database.DB }

const scanCols = `SELECT id, owner_id, COALESCE(field_id::text, ''), media_id, crop_code, phash, status, idempotency_key,
	captured_at, created_at, updated_at, note, saved, diagnosis FROM diagnosis_scans`

func scanRow(row pgx.CollectableRow) (domain.Scan, error) {
	var s domain.Scan
	var phash int64
	var status string
	var diag []byte
	err := row.Scan(&s.ID, &s.OwnerID, &s.FieldID, &s.MediaID, &s.CropCode, &phash, &status, &s.IdempotencyKey,
		&s.CapturedAt, &s.CreatedAt, &s.UpdatedAt, &s.Note, &s.Saved, &diag)
	s.PHash, s.Status = uint64(phash), domain.Status(status) //nolint:gosec // bit pattern
	s.CapturedAt, s.CreatedAt, s.UpdatedAt = s.CapturedAt.UTC(), s.CreatedAt.UTC(), s.UpdatedAt.UTC()
	if diag != nil {
		s.Diagnosis = &domain.Diagnosis{}
		_ = json.Unmarshal(diag, s.Diagnosis) // written by SaveResult from the same type
	}
	return s, err
}

func one(ss []domain.Scan, err error) (domain.Scan, error) {
	if err == nil && len(ss) == 0 {
		err = domain.ErrNotFound
	}
	if err != nil {
		return domain.Scan{}, err
	}
	return ss[0], nil
}

// Create inserts a queued scan.
func (r Scans) Create(ctx context.Context, s domain.Scan) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO diagnosis_scans (id, owner_id, field_id, media_id, crop_code, phash, status,
		idempotency_key, captured_at, created_at, updated_at) VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7, $8, $9, $10, $11)`,
		s.ID, s.OwnerID, s.FieldID, s.MediaID, s.CropCode, int64(s.PHash), string(s.Status), s.IdempotencyKey, //nolint:gosec // bit pattern
		s.CapturedAt, s.CreatedAt, s.UpdatedAt)
	return err
}

// Get loads a scan.
func (r Scans) Get(ctx context.Context, id string) (domain.Scan, error) {
	return one(database.Collect(ctx, r.DB.Q(ctx), scanRow, scanCols+` WHERE id = $1`, id))
}

// GetByKey loads a scan by its owner-scoped idempotency key.
func (r Scans) GetByKey(ctx context.Context, ownerID, key string) (domain.Scan, error) {
	return one(database.Collect(ctx, r.DB.Q(ctx), scanRow, scanCols+` WHERE owner_id = $1 AND idempotency_key = $2`, ownerID, key))
}

// List returns scans newest first with keyset pagination on created_at.
func (r Scans) List(ctx context.Context, f domain.ListFilter) ([]domain.Scan, error) {
	before := f.Before
	if before.IsZero() {
		before = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return database.Collect(ctx, r.DB.Q(ctx), scanRow, scanCols+` WHERE owner_id = $1 AND created_at < $2 AND (NOT $3 OR saved)
		ORDER BY created_at DESC, id DESC LIMIT $4`, f.OwnerID, before, f.SavedOnly, f.Limit)
}

// SaveResult persists status and diagnosis.
func (r Scans) SaveResult(ctx context.Context, s domain.Scan) error {
	var diag []byte
	if s.Diagnosis != nil {
		diag, _ = json.Marshal(s.Diagnosis) // plain struct of strings/numbers
	}
	_, err := r.DB.Q(ctx).Exec(ctx, `UPDATE diagnosis_scans SET status = $2, diagnosis = $3, updated_at = $4 WHERE id = $1`,
		s.ID, string(s.Status), diag, s.UpdatedAt)
	return err
}

// Annotate stores the note / saved flag.
func (r Scans) Annotate(ctx context.Context, id string, a domain.Annotation) error {
	_, err := r.DB.Q(ctx).Exec(ctx, `UPDATE diagnosis_scans SET note = $2, saved = $3, updated_at = $4 WHERE id = $1`, id, a.Note, a.Saved, a.UpdatedAt)
	return err
}

// RecordConflict preserves an overwritten annotation.
func (r Scans) RecordConflict(ctx context.Context, id, scanID string, loser, winner domain.Annotation, at time.Time) error {
	l, _ := json.Marshal(loser)
	w, _ := json.Marshal(winner)
	_, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO diagnosis_sync_audit (id, scan_id, loser, winner, received_at) VALUES ($1, $2, $3, $4, $5)`,
		id, scanID, l, w, at)
	return err
}

// ClaimOp stores an operation outcome once.
func (r Scans) ClaimOp(ctx context.Context, ownerID string, o domain.SyncOutcome, at time.Time) (bool, error) {
	tag, err := r.DB.Q(ctx).Exec(ctx, `INSERT INTO diagnosis_sync_ops (owner_id, idempotency_key, scan_id, outcome, error_code, received_at)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`, ownerID, o.IdempotencyKey, o.ScanID, o.Outcome, o.ErrorCode, at)
	return tag.RowsAffected() == 1, err
}

// GetOp loads a processed operation.
func (r Scans) GetOp(ctx context.Context, ownerID, key string) (domain.SyncOutcome, error) {
	var o domain.SyncOutcome
	err := r.DB.Q(ctx).QueryRow(ctx, `SELECT idempotency_key, scan_id, outcome, error_code FROM diagnosis_sync_ops
		WHERE owner_id = $1 AND idempotency_key = $2`, ownerID, key).Scan(&o.IdempotencyKey, &o.ScanID, &o.Outcome, &o.ErrorCode)
	return o, database.NotFound(err, domain.ErrNotFound)
}
