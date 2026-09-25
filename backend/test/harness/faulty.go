package harness

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
)

// Fault makes one operation fail. Op is "exec", "query", "queryrow" or "begin";
// Match is a substring of the SQL (ignored for "begin"). Skip lets that many
// matching calls succeed first.
type Fault struct {
	Op    string
	Match string
	Skip  int
	Err   error
}

type faults struct{ list []*Fault }

func (f *faults) hit(op, sql string) error {
	for _, ft := range f.list {
		if ft.Op == op && strings.Contains(sql, ft.Match) {
			if ft.Skip > 0 {
				ft.Skip--
				continue
			}
			return ft.Err
		}
	}
	return nil
}

type faultyTx struct {
	pgx.Tx
	f *faults
}

// Faulty wraps tx so matching operations (including inside nested savepoints)
// return the configured errors.
func Faulty(tx pgx.Tx, fs ...Fault) pgx.Tx {
	f := &faults{}
	for i := range fs {
		f.list = append(f.list, &fs[i])
	}
	return faultyTx{Tx: tx, f: f}
}

// FaultyDB is DB with fault injection.
func FaultyDB(t testing.TB, fs ...Fault) *database.DB {
	t.Helper()
	return database.New(Faulty(Tx(t), fs...))
}

func (t faultyTx) Begin(ctx context.Context) (pgx.Tx, error) {
	if err := t.f.hit("begin", ""); err != nil {
		return nil, err
	}
	inner, err := t.Tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return faultyTx{Tx: inner, f: t.f}, nil
}

func (t faultyTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if err := t.f.hit("exec", sql); err != nil {
		return pgconn.CommandTag{}, err
	}
	return t.Tx.Exec(ctx, sql, args...)
}

func (t faultyTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if err := t.f.hit("query", sql); err != nil {
		return nil, err
	}
	return t.Tx.Query(ctx, sql, args...)
}

func (t faultyTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if err := t.f.hit("queryrow", sql); err != nil {
		return errRow{err}
	}
	return t.Tx.QueryRow(ctx, sql, args...)
}

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }
