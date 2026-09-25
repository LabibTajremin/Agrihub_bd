//go:build integration

package harness

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeTB struct {
	testing.TB
	failed bool
}

func (f *fakeTB) Helper()           {}
func (f *fakeTB) Fatal(args ...any) { f.failed = true }

func TestMust(t *testing.T) {
	f := &fakeTB{TB: t}
	if !must(f, nil) || f.failed {
		t.Fatal("nil error must pass")
	}
	if must(f, errors.New("x")) || !f.failed {
		t.Fatal("error must fail the test")
	}
	saved := pool
	pool = &lazy[*pgxpool.Pool]{fn: func() (*pgxpool.Pool, error) { return nil, errors.New("down") }}
	defer func() { pool = saved }()
	if Tx(f) != nil {
		t.Fatal("tx after failure path must be nil")
	}
}

func TestStartPostgres_BadImageFails(t *testing.T) {
	if _, err := StartPostgres(context.Background(), "agrismart-invalid/none:0"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDB_RollsBackAfterTest(t *testing.T) {
	ctx := context.Background()
	t.Run("write", func(t *testing.T) {
		db := DB(t)
		if _, err := db.Q(ctx).Exec(ctx, "CREATE TABLE scratch (id int)"); err != nil {
			t.Fatal(err)
		}
	})
	var n int
	err := Pool(t).QueryRow(ctx, "SELECT count(*) FROM pg_tables WHERE tablename = 'scratch'").Scan(&n)
	if err != nil || n != 0 {
		t.Fatal("per-test transaction must roll back", n, err)
	}
	if NewDatabaseDSN(t) == "" {
		t.Fatal("dsn")
	}
}

func TestCreateDatabase_ServerErrorPropagates(t *testing.T) {
	failing := &lazy[string]{fn: func() (string, error) { return "", errors.New("no docker") }}
	saved := server
	server = failing
	defer func() { server = saved }()
	if _, err := createDatabase(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if _, err := migratedPool(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateDatabase_ConnectAndCreateErrors(t *testing.T) {
	saved := server
	defer func() { server = saved }()
	server = &lazy[string]{fn: func() (string, error) { return "postgres://agri:agri@127.0.0.1:1/agri?connect_timeout=1", nil }}
	if _, err := createDatabase(context.Background()); err == nil {
		t.Fatal("connect must fail")
	}
	base, _ := saved.get()
	server = &lazy[string]{fn: func() (string, error) {
		return base + "&options=-c%20default_transaction_read_only%3Don", nil
	}}
	if _, err := createDatabase(context.Background()); err == nil {
		t.Fatal("CREATE DATABASE in a read-only session must fail")
	}
}

func TestMigratedPool_MigrationErrorPropagates(t *testing.T) {
	bad := fstest.MapFS{"1_bad.up.sql": {Data: []byte("THIS IS NOT SQL")}}
	if _, err := migratedPool(bad); err == nil {
		t.Fatal("a broken migration must fail")
	}
}

func TestFaulty(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")
	tx := Faulty(Tx(t),
		Fault{Op: "exec", Match: "SELECT 1", Skip: 1, Err: boom},
		Fault{Op: "query", Match: "SELECT 2", Err: boom},
		Fault{Op: "queryrow", Match: "SELECT 3", Err: boom},
	)
	if _, err := tx.Exec(ctx, "SELECT 1"); err != nil {
		t.Fatal("first matching exec is skipped", err)
	}
	if _, err := tx.Exec(ctx, "SELECT 1"); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	inner, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inner.Query(ctx, "SELECT 2"); !errors.Is(err, boom) {
		t.Fatal("faults apply inside savepoints", err)
	}
	rows, err := inner.Query(ctx, "SELECT 4")
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()
	var n int
	if err := inner.QueryRow(ctx, "SELECT 3").Scan(&n); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if err := inner.QueryRow(ctx, "SELECT 5").Scan(&n); err != nil || n != 5 {
		t.Fatal(err)
	}
	over := FaultyOver(DB(t), Fault{Op: "exec", Match: "SELECT 9", Err: boom})
	if _, err := over.Q(ctx).Exec(ctx, "SELECT 9"); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	db := FaultyDB(t, Fault{Op: "begin", Err: boom})
	if err := db.WithinTx(ctx, func(context.Context) error { return nil }); err == nil {
		t.Fatal("begin fault")
	}
	_ = inner.Rollback(ctx)
	if _, err := inner.Begin(ctx); err == nil {
		t.Fatal("begin on a closed savepoint must fail")
	}
}
