package settings

import (
	"net/http"
	"slices"
	"strings"
	"unicode"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Izin yang hanya dimiliki Super Admin (peran sistem): tidak boleh masuk peran kustom agar tidak ada jalur
// menaikkan hak akses lewat peran buatan sendiri.
var reservedPerms = []string{auth.PermRolesManage, auth.PermCredentialsManage}

type roleReq struct {
	Name            string   `json:"name"`
	Permissions     []string `json:"permissions"`
	ExpectedVersion int32    `json:"expectedVersion"`
}

func validateRole(req *roleReq) map[string]string {
	f := map[string]string{}
	req.Name = strings.TrimSpace(req.Name)
	if len([]rune(req.Name)) < 3 || len([]rune(req.Name)) > 60 {
		f["name"] = "Nama peran 3 sampai 60 karakter."
	}
	if len(req.Permissions) == 0 {
		f["permissions"] = "Pilih minimal satu izin."
	}
	seen := map[string]bool{}
	for _, p := range req.Permissions {
		if _, ok := auth.PermissionCatalog[p]; !ok {
			f["permissions"] = "Izin tidak dikenal: " + p
		} else if slices.Contains(reservedPerms, p) {
			f["permissions"] = "Izin \"" + auth.PermissionCatalog[p] + "\" hanya untuk Super Admin."
		}
		seen[p] = true
	}
	req.Permissions = req.Permissions[:0]
	for p := range seen {
		req.Permissions = append(req.Permissions, p)
	}
	slices.Sort(req.Permissions)
	return f
}

// roleKey membuat kunci dari nama (huruf kecil dan garis bawah), dengan akhiran angka bila sudah dipakai.
func (h *Handler) roleKey(c *gin.Context, q *sqlc.Queries, name string) (string, error) {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '-' || r == '_':
			if s := b.String(); s != "" && !strings.HasSuffix(s, "_") {
				b.WriteByte('_')
			}
		}
	}
	base := strings.Trim(b.String(), "_")
	if len(base) < 2 {
		base = "peran"
	}
	if len(base) > 34 {
		base = strings.TrimRight(base[:34], "_")
	}
	// Kunci hanya boleh huruf dan garis bawah (constraint tabel), jadi akhiran pembeda memakai huruf: _a, _b, ...
	key := base
	for suffix := 'a'; ; suffix++ {
		exists, err := q.RoleKeyExists(c.Request.Context(), sqlc.RoleKeyExistsParams{OrgID: h.OrgID, Key: key})
		if err != nil || !exists {
			return key, err
		}
		if suffix > 'z' {
			return "", platform.ErrConflict("role_key_exhausted", "Terlalu banyak peran dengan nama serupa. Gunakan nama lain.")
		}
		key = base + "_" + string(suffix)
	}
}

// POST /admin/roles (roles.manage): peran kustom baru.
func (h *Handler) CreateRole(c *gin.Context) {
	var req roleReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if f := validateRole(&req); len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	ctx := c.Request.Context()
	var role sqlc.Role
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		key, err := h.roleKey(c, q, req.Name)
		if err != nil {
			return err
		}
		if role, err = q.InsertRole(ctx, sqlc.InsertRoleParams{OrgID: h.OrgID, Key: key, Name: req.Name}); err != nil {
			return err
		}
		for _, p := range req.Permissions {
			if err := q.InsertRolePermission(ctx, sqlc.InsertRolePermissionParams{RoleID: role.ID, PermissionKey: p}); err != nil {
				return err
			}
		}
		return h.audit(ctx, q, c, "admin.role_create", "role", key, "", map[string]any{"name": req.Name, "permissions": req.Permissions})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"key": role.Key, "version": role.Version})
}

// PUT /admin/roles/:key (roles.manage): ubah nama dan izin peran kustom. Berlaku pada permintaan berikutnya
// semua pemegang peran (hak akses dibaca ulang tiap request).
func (h *Handler) UpdateRole(c *gin.Context) {
	var req roleReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if f := validateRole(&req); len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	ctx := c.Request.Context()
	var out sqlc.Role
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		role, err := h.lockCustomRole(c, q)
		if err != nil {
			return err
		}
		if role.Version != req.ExpectedVersion {
			return platform.ErrConflict("stale_version", "Peran sudah diubah orang lain. Muat ulang untuk melihat versi terbaru.")
		}
		before, err := q.ListRolesWithPermissions(ctx, h.OrgID)
		if err != nil {
			return err
		}
		var old []string
		for _, r := range before {
			if r.ID == role.ID {
				old = r.Permissions
			}
		}
		if err := q.DeleteRolePermissions(ctx, role.ID); err != nil {
			return err
		}
		for _, p := range req.Permissions {
			if err := q.InsertRolePermission(ctx, sqlc.InsertRolePermissionParams{RoleID: role.ID, PermissionKey: p}); err != nil {
				return err
			}
		}
		if out, err = q.UpdateRole(ctx, sqlc.UpdateRoleParams{ID: role.ID, Name: req.Name}); err != nil {
			return err
		}
		return h.audit(ctx, q, c, "admin.role_update", "role", role.Key, "", map[string]any{"name": req.Name, "before": old, "after": req.Permissions})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": out.Key, "version": out.Version})
}

// POST /admin/roles/:key/archive (roles.manage): hanya bila tidak ada penugasan aktif.
func (h *Handler) ArchiveRole(c *gin.Context) {
	ctx := c.Request.Context()
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		role, err := h.lockCustomRole(c, q)
		if err != nil {
			return err
		}
		n, err := q.CountActiveRoleAssignments(ctx, role.ID)
		if err != nil {
			return err
		}
		if n > 0 {
			return platform.ErrConflict("role_in_use", "Peran masih dipakai pengguna. Cabut semua penugasannya terlebih dahulu.")
		}
		if err := q.ArchiveRole(ctx, role.ID); err != nil {
			return err
		}
		return h.audit(ctx, q, c, "admin.role_archive", "role", role.Key, "", nil)
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) lockCustomRole(c *gin.Context, q *sqlc.Queries) (sqlc.Role, error) {
	role, err := q.LockRole(c.Request.Context(), sqlc.LockRoleParams{OrgID: h.OrgID, Key: c.Param("key")})
	if platform.IsNoRows(err) || (err == nil && role.ArchivedAt != nil) {
		return role, platform.ErrNotFound("Peran tidak ditemukan.")
	}
	if err != nil {
		return role, err
	}
	if role.System {
		return role, platform.ErrConflict("system_role", "Peran bawaan tidak dapat diubah. Buat peran baru dengan izin yang diinginkan.")
	}
	return role, nil
}
