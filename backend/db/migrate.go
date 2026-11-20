package db

import (
	"errors"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// MigrateUp menerapkan semua migrasi tertanam. Mengembalikan versi akhir dan status dirty.
func MigrateUp(dsn string) (uint, bool, error) {
	src, err := iofs.New(Migrations, "migrations")
	if err != nil {
		return 0, false, err
	}
	url := dsn
	for _, p := range []string{"postgres://", "postgresql://"} {
		if rest, ok := strings.CutPrefix(url, p); ok {
			url = "pgx5://" + rest
		}
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, url)
	if err != nil {
		return 0, false, err
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, false, err
	}
	v, dirty, err := m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		err = nil
	}
	return v, dirty, err
}
