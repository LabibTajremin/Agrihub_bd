// Package database owns Postgres access: a pooler-aware pgx pool, a context
// propagated transaction manager with savepoint nesting, migrations and small
// repository helpers.
package database

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// Querier is the subset of pgx shared by pools and transactions.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Beginner starts transactions (pool or tx for savepoints).
type Beginner interface {
	Querier
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Options configures the pool.
type Options struct {
	URL            string
	PoolMode       string // "transaction" (pgbouncer/serverless) or "session"
	MaxConns       int32
	MinConns       int32
	ConnectTimeout time.Duration
}

// PoolConfig translates Options into a pgxpool config. In transaction pool mode
// server-side prepared statement caching is disabled, which is what
// pgbouncer/Neon/Supabase poolers require.
func PoolConfig(o Options) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(o.URL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = o.MaxConns
	cfg.MinConns = o.MinConns
	cfg.ConnConfig.ConnectTimeout = o.ConnectTimeout
	if o.PoolMode == "transaction" {
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
		cfg.ConnConfig.StatementCacheCapacity = 0
		cfg.ConnConfig.DescriptionCacheCapacity = 0
	}
	return cfg, nil
}

// ErrUnavailable is returned when a transaction cannot start.
var ErrUnavailable = errs.Unavailable("database.unavailable")

// DB is the database handle used by repositories.
type DB struct {
	root Beginner
}

// New wraps an existing Beginner (pool or, in tests, an outer transaction).
func New(root Beginner) *DB { return &DB{root: root} }

// Open creates a pool and verifies connectivity.
func Open(ctx context.Context, o Options) (*DB, *pgxpool.Pool, error) {
	cfg, err := PoolConfig(o)
	if err != nil {
		return nil, nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, err
	}
	return New(pool), pool, nil
}

type txKey struct{}

// WithTx returns ctx carrying tx; repositories then run inside it.
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

func txFrom(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

// Q returns the active transaction from ctx, or the root handle.
func (d *DB) Q(ctx context.Context) Querier {
	if tx, ok := txFrom(ctx); ok {
		return tx
	}
	return d.root
}

// WithinTx implements tx.Manager. Nested calls create a savepoint.
func (d *DB) WithinTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	parent := d.root
	if tx, ok := txFrom(ctx); ok {
		parent = tx
	}
	tx, err := parent.Begin(ctx)
	if err != nil {
		return ErrUnavailable.Wrap(err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	if err = fn(WithTx(ctx, tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// IsUniqueViolation reports a unique-constraint violation (SQLSTATE 23505).
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// NotFound maps pgx.ErrNoRows to notFound, passing other errors through.
func NotFound(err, notFound error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound
	}
	return err
}

// MigrateURL converts a postgres:// DSN to the pgx5:// scheme golang-migrate expects.
func MigrateURL(dsn string) string {
	for _, p := range []string{"postgres://", "postgresql://"} {
		if rest, ok := strings.CutPrefix(dsn, p); ok {
			return "pgx5://" + rest
		}
	}
	return dsn
}

// Collect runs a query and scans every row with scan, closing rows and
// surfacing scan/iteration errors through a single return.
func Collect[T any](ctx context.Context, q Querier, scan func(pgx.CollectableRow) (T, error), sql string, args ...any) ([]T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scan)
}

// Root returns the handle queries fall back to outside a transaction.
func (d *DB) Root() Beginner { return d.root }
