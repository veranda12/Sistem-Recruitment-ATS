// Perintah migrate menjalankan migrasi tertanam. Sengaja tidak menyediakan "down": rollback rilis memakai
// binary kompatibel + migrasi expand/contract, bukan down migration destruktif otomatis.
package main

import (
	"fmt"
	"os"

	"ats/db"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL wajib diisi")
		os.Exit(2)
	}
	if len(os.Args) > 1 && os.Args[1] != "up" {
		fmt.Fprintf(os.Stderr, "perintah tidak dikenal %q (gunakan: up)\n", os.Args[1])
		os.Exit(2)
	}
	v, dirty, err := db.MigrateUp(dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrasi gagal:", err)
		os.Exit(1)
	}
	fmt.Printf("versi skema: %d dirty=%v\n", v, dirty)
	if dirty {
		os.Exit(1)
	}
}
