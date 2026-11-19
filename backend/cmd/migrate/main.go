// Perintah migrate menjalankan migrasi tertanam. Sengaja tidak menyediakan "down": rollback rilis memakai
// binary kompatibel + migrasi expand/contract, bukan down migration destruktif otomatis.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"ats/db"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL wajib diisi")
		os.Exit(2)
	}
	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	src, err := iofs.New(db.Migrations, "migrations")
	if err != nil {
		fail(err)
	}
	// Driver pgx/v5 memakai skema URL pgx5://
	url := dsn
	for _, p := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(url, p) {
			url = "pgx5://" + strings.TrimPrefix(url, p)
		}
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, url)
	if err != nil {
		fail(err)
	}
	defer m.Close()
	switch cmd {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			fail(err)
		}
	case "version":
	default:
		fail(fmt.Errorf("perintah tidak dikenal %q (gunakan: up | version)", cmd))
	}
	v, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		fail(err)
	}
	fmt.Printf("versi skema: %d dirty=%v\n", v, dirty)
	if dirty {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "migrasi gagal:", err)
	os.Exit(1)
}
