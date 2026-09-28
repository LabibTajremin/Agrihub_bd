// Package harness is the integration-test harness: one disposable Postgres
// (testcontainers) per test binary, one freshly migrated database per binary,
// and one rolled-back transaction per test. It also offers fault injection so
// repository error paths are testable against a real database.
package harness

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"net/url"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/migrations"
)

// Image is the Postgres image used for integration tests.
const Image = "postgres:16-alpine"

// process-wide fixtures: one container and one migrated database per test binary.
var (
	server = &lazy[string]{fn: func() (string, error) { return StartPostgres(context.Background(), Image) }}
	pool   = &lazy[*pgxpool.Pool]{fn: func() (*pgxpool.Pool, error) { return migratedPool(migrations.FS) }}
)

type lazy[T any] struct {
	once sync.Once
	fn   func() (T, error)
	val  T
	err  error
}

func (l *lazy[T]) get() (T, error) {
	l.once.Do(func() { l.val, l.err = l.fn() })
	return l.val, l.err
}

// StartPostgres runs a Postgres container and returns its DSN. The container
// is reaped by testcontainers when the test process exits.
func StartPostgres(ctx context.Context, image string) (string, error) {
	c, err := postgres.Run(ctx, image,
		postgres.WithDatabase("agri"), postgres.WithUsername("agri"), postgres.WithPassword("agri"),
		postgres.BasicWaitStrategies())
	if err != nil {
		return "", err
	}
	return c.ConnectionString(ctx, "sslmode=disable")
}

// must fails the test on err. It is the single failure branch of the harness.
func must(t testing.TB, err error) bool {
	t.Helper()
	if err != nil {
		t.Fatal(err)
		return false
	}
	return true
}

// NewDatabaseDSN creates an empty, uniquely named database and returns its DSN.
func NewDatabaseDSN(t testing.TB) string {
	t.Helper()
	dsn, err := createDatabase(context.Background())
	must(t, err)
	return dsn
}

func createDatabase(ctx context.Context) (string, error) {
	base, err := server.get()
	if err != nil {
		return "", err
	}
	name := "t_" + randomHex()
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		return "", err
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		return "", err
	}
	u, _ := url.Parse(base) // base comes from testcontainers and is a valid URL
	u.Path = "/" + name
	return u.String(), nil
}

func randomHex() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func migratedPool(fsys fs.FS) (*pgxpool.Pool, error) {
	ctx := context.Background()
	dsn, err := createDatabase(ctx)
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(dsn, fsys, "up"); err != nil {
		return nil, err
	}
	// Same pool mode as production (pgbouncer-safe exec mode), so the suite
	// catches encoding differences such as []byte → jsonb.
	cfg, _ := database.PoolConfig(database.Options{URL: dsn, PoolMode: "transaction", MaxConns: MaxConns}) // dsn from createDatabase
	return pgxpool.NewWithConfig(ctx, cfg)
}

// MaxConns bounds the shared pool; each harness.DB holds one connection until
// its test ends, so tests may open many.
const MaxConns = 60

// Pool returns the process-wide migrated pool.
func Pool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	p, err := pool.get()
	must(t, err)
	return p
}

// Tx begins a transaction that is rolled back when the test ends.
func Tx(t testing.TB) pgx.Tx {
	t.Helper()
	tx, err := beginTx(pool)
	if !must(t, err) {
		return nil
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

// DB returns a database handle whose every statement runs inside the test's
// rolled-back transaction; nested WithinTx calls become savepoints.
func DB(t testing.TB) *database.DB {
	t.Helper()
	return database.New(Tx(t))
}

func beginTx(l *lazy[*pgxpool.Pool]) (pgx.Tx, error) {
	p, err := l.get()
	if err != nil {
		return nil, err
	}
	return p.Begin(context.Background())
}
