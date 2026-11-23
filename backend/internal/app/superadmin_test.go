package app

import (
	"context"
	"testing"
)

// Super Admin memegang semua izin, termasuk izin yang ditambahkan kemudian (trigger migrasi 0015).
func TestSuperAdminHoldsAllPermissions(t *testing.T) {
	e := newEnv(t)
	missing := `SELECT count(*) FROM permissions p WHERE NOT EXISTS (
		SELECT 1 FROM role_permissions rp JOIN roles r ON r.id = rp.role_id
		WHERE r.system AND r.key = 'super_admin' AND rp.permission_key = p.key)`
	if n := e.count(missing); n != 0 {
		t.Fatalf("Super Admin belum memiliki %d izin", n)
	}
	admin := e.client().mustLogin("admin@arunika.example")
	expect(t, admin.do("GET", "/applications", nil), 200, "daftar lamaran")
	expect(t, admin.do("GET", "/exam-banks", nil), 200, "bank soal")
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO permissions (key, description) VALUES ('uji.izin_baru', 'Izin uji')`); err != nil {
		t.Fatal(err)
	}
	if n := e.count(missing); n != 0 {
		t.Fatal("izin baru tidak otomatis diberikan ke Super Admin")
	}
}
