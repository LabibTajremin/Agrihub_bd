package database

import (
	"errors"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // pgx5:// driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrate applies ("up") or reverts ("down") every migration in fsys (root dir ".").
func Migrate(dsn string, fsys fs.FS, direction string) error {
	src, err := iofs.New(fsys, ".")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, MigrateURL(dsn))
	if err != nil {
		return err
	}
	defer m.Close()
	switch direction {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	default:
		return errors.New("database: direction must be up or down")
	}
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}
