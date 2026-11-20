// Seed data fiktif PT Arunika Daya Persada untuk development, test, dan staging.
// Menolak berjalan di produksi. Kata sandi akun seed diambil dari SEED_PASSWORD (tidak ada sandi default).
package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"ats/db"
	"ats/db/sqlc"
	"ats/internal/app"
	"ats/internal/auth"
	"ats/internal/identity"
	"ats/internal/platform"

	"github.com/jackc/pgx/v5"
)

func main() {
	cfg, err := platform.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if cfg.IsProduction() {
		fmt.Fprintln(os.Stderr, "seed ditolak: APP_ENV=production")
		os.Exit(2)
	}
	pw := os.Getenv("SEED_PASSWORD")
	if auth.ValidatePassword(pw) != "" {
		fmt.Fprintln(os.Stderr, "SEED_PASSWORD wajib diisi (min 10 karakter, huruf dan angka)")
		os.Exit(2)
	}
	ctx := context.Background()
	if len(os.Args) > 1 && os.Args[1] == "--reset" {
		if err := reset(ctx, cfg); err != nil {
			fmt.Fprintln(os.Stderr, "reset ditolak/gagal:", err)
			os.Exit(2)
		}
	}
	pool, err := platform.NewPool(ctx, cfg.DatabaseURL, 4, "ats-seed")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer pool.Close()
	if _, err := sqlc.New(pool).GetOrganizationBySlug(ctx, cfg.OrgSlug); err == nil {
		fmt.Println("seed dilewati: organisasi", cfg.OrgSlug, "sudah ada")
		return
	}
	keys := identity.Keys{EncKey: cfg.NIKEncKey, EncVersion: cfg.NIKEncKeyVer, IndexKey: cfg.NIKIndexKey, IndexVersion: cfg.NIKIndexKeyVer}
	hash, err := auth.HashPassword(ctx, pw)
	if err != nil {
		panic(err)
	}
	err = platform.WithTx(ctx, pool, func(tx pgx.Tx) error { return app.Seed(ctx, tx, cfg.OrgSlug, hash, keys) })
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed gagal:", err)
		os.Exit(1)
	}
	fmt.Println("seed selesai. Akun (sandi = SEED_PASSWORD):")
	for _, u := range app.SeedUsers {
		fmt.Printf("  %-28s %s\n", u.Email, u.Label)
	}
}

// reset menghapus skema public lalu menerapkan migrasi. Hanya untuk APP_ENV development/test dan
// database yang namanya berakhiran _dev, _test, atau _e2e, agar tidak pernah menyentuh data nyata.
func reset(ctx context.Context, cfg platform.Config) error {
	if cfg.Env != "development" && cfg.Env != "test" {
		return errors.New("hanya untuk APP_ENV development atau test")
	}
	u, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	name := strings.TrimPrefix(u.Path, "/")
	if !strings.HasSuffix(name, "_dev") && !strings.HasSuffix(name, "_test") && !strings.HasSuffix(name, "_e2e") {
		return fmt.Errorf("nama database %q harus berakhiran _dev, _test, atau _e2e", name)
	}
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;"); err != nil {
		conn.Close(ctx)
		return err
	}
	conn.Close(ctx)
	v, _, err := db.MigrateUp(cfg.DatabaseURL)
	if err == nil {
		fmt.Printf("database %s direset, versi skema %d\n", name, v)
	}
	return err
}
