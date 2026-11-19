// Package db menanam file migrasi agar binary migrate tidak bergantung pada path checkout.
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
