//go:build integration

package database_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/migrations"
	"github.com/labibtajremin/agrihub_bd/backend/test/harness"
)

var ctx = context.Background()

func TestPoolConfig_PoolerAware(t *testing.T) {
	if _, err := database.PoolConfig(database.Options{URL: "::bad"}); err == nil {
		t.Fatal("bad url")
	}
	cfg, err := database.PoolConfig(database.Options{URL: "postgres://a@b/c", PoolMode: "transaction", MaxConns: 4, ConnectTimeout: time.Second})
	if err != nil || cfg.ConnConfig.DefaultQueryExecMode != pgx.QueryExecModeExec || cfg.ConnConfig.StatementCacheCapacity != 0 || cfg.MaxConns != 4 {
		t.Fatal(cfg, err)
	}
	cfg, _ = database.PoolConfig(database.Options{URL: "postgres://a@b/c", PoolMode: "session", MaxConns: 1})
	if cfg.ConnConfig.DefaultQueryExecMode == pgx.QueryExecModeExec {
		t.Fatal("session mode keeps statement caching")
	}
}

func TestOpen(t *testing.T) {
	dsn := harness.NewDatabaseDSN(t)
	db, pool, err := database.Open(ctx, database.Options{URL: dsn, PoolMode: "transaction", MaxConns: 2, ConnectTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var one int
	if err := db.Q(ctx).QueryRow(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatal(err)
	}
	cases := []database.Options{
		{URL: "::bad"},
		{URL: dsn, MaxConns: 0},
		{URL: "postgres://agri:agri@127.0.0.1:1/agri", MaxConns: 1, ConnectTimeout: time.Second},
	}
	for _, o := range cases {
		if _, _, err := database.Open(ctx, o); err == nil {
			t.Errorf("%+v: expected error", o)
		}
	}
}

func TestWithinTx_CommitRollbackAndSavepoints(t *testing.T) {
	db := harness.DB(t)
	q := db.Q(ctx)
	if _, err := q.Exec(ctx, "CREATE TABLE t (v int PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	err := db.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := db.Q(ctx).Exec(ctx, "INSERT INTO t VALUES (1)"); err != nil {
			return err
		}
		inner := db.WithinTx(ctx, func(ctx context.Context) error {
			_, _ = db.Q(ctx).Exec(ctx, "INSERT INTO t VALUES (2)")
			return boom
		})
		if !errors.Is(inner, boom) {
			t.Fatal(inner)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.WithinTx(ctx, func(ctx context.Context) error {
		_, _ = db.Q(ctx).Exec(ctx, "INSERT INTO t VALUES (3)")
		return boom
	})
	vals, err := database.Collect(ctx, q, pgx.RowTo[int], "SELECT v FROM t ORDER BY v")
	if err != nil || len(vals) != 1 || vals[0] != 1 {
		t.Fatalf("only the committed outer write survives: %v %v", vals, err)
	}
	_, err = q.Exec(ctx, "INSERT INTO t VALUES (1)")
	if !database.IsUniqueViolation(err) || database.IsUniqueViolation(boom) {
		t.Fatal(err)
	}
}

func TestWithinTx_BeginFailureIsUnavailable(t *testing.T) {
	db := harness.FaultyDB(t, harness.Fault{Op: "begin", Err: errors.New("down")})
	err := db.WithinTx(ctx, func(context.Context) error { return nil })
	if err == nil || err.Error() != "database.unavailable: down" {
		t.Fatal(err)
	}
}

func TestCollect_QueryError(t *testing.T) {
	db := harness.FaultyDB(t, harness.Fault{Op: "query", Match: "SELECT", Err: errors.New("x")})
	if _, err := database.Collect(ctx, db.Q(ctx), pgx.RowTo[int], "SELECT 1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestHelpers(t *testing.T) {
	nf := errors.New("nf")
	if !errors.Is(database.NotFound(pgx.ErrNoRows, nf), nf) {
		t.Fatal()
	}
	other := errors.New("o")
	if database.NotFound(other, nf) != other {
		t.Fatal()
	}
	for in, want := range map[string]string{
		"postgres://a@b/c":   "pgx5://a@b/c",
		"postgresql://a@b/c": "pgx5://a@b/c",
		"pgx5://x":           "pgx5://x",
	} {
		if database.MigrateURL(in) != want {
			t.Error(in)
		}
	}
}

func TestMigrate_UpDownUpCleanly(t *testing.T) {
	dsn := harness.NewDatabaseDSN(t)
	for _, dir := range []string{"up", "up", "down", "up"} {
		if err := database.Migrate(dsn, migrations.FS, dir); err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
	}
	if err := database.Migrate(dsn, migrations.FS, "sideways"); err == nil {
		t.Fatal("bad direction")
	}
	if err := database.Migrate(dsn, fstest.MapFS{}, "up"); err == nil {
		t.Fatal("empty source")
	}
	if err := database.Migrate(dsn, deniedFS{}, "up"); err == nil {
		t.Fatal("unreadable source")
	}
	if err := database.Migrate("postgres://agri:agri@127.0.0.1:1/x?connect_timeout=1", migrations.FS, "up"); err == nil {
		t.Fatal("unreachable db")
	}
}

type deniedFS struct{}

func (deniedFS) Open(string) (fs.File, error) { return nil, fs.ErrPermission }
