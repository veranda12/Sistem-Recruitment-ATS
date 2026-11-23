// Package settings: pengguna internal, penugasan peran berscope, audit read-only, pengaturan berversi,
// profil organisasi/brand, master data, dan operasi dead-letter outbox.
package settings

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	Pool             *pgxpool.Pool
	Q                *sqlc.Queries
	OrgID            uuid.UUID
	InvalidatePublic func(context.Context)
}

func (h *Handler) audit(ctx context.Context, q *sqlc.Queries, c *gin.Context, action, ttype, tid, reason string, detail map[string]any) error {
	p := auth.Current(c)
	return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: action,
		TargetType: ttype, TargetID: tid, Reason: reason, RequestID: platform.RequestID(c), Detail: detail})
}

func pathID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound(""))
	}
	return id, err == nil
}

// ---- Pengguna dan peran ----

// GET /admin/users
func (h *Handler) ListUsers(c *gin.Context) {
	ctx := c.Request.Context()
	users, err := h.Q.ListStaffUsers(ctx, h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	asg, err := h.Q.ListAssignments(ctx, h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	byUser := map[uuid.UUID][]sqlc.ListAssignmentsRow{}
	for _, a := range asg {
		byUser[a.UserID] = append(byUser[a.UserID], a)
	}
	type item struct {
		sqlc.ListStaffUsersRow
		Assignments []sqlc.ListAssignmentsRow `json:"assignments"`
	}
	out := []item{}
	for _, u := range users {
		a := byUser[u.ID]
		if a == nil {
			a = []sqlc.ListAssignmentsRow{}
		}
		out = append(out, item{u, a})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// GET /admin/roles
func (h *Handler) ListRoles(c *gin.Context) {
	rows, err := h.Q.ListRolesWithPermissions(c.Request.Context(), h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "catalog": auth.PermissionCatalog})
}

type createUserReq struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// POST /admin/users: akun staf tanpa sandi default; undangan dikirim sebagai tautan atur sandi sekali pakai.
func (h *Handler) CreateUser(c *gin.Context) {
	var req createUserReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	f := map[string]string{}
	email, ok := auth.NormalizeEmail(req.Email)
	if !ok {
		f["email"] = "Masukkan email yang valid."
	}
	req.Name = strings.TrimSpace(req.Name)
	if len(req.Name) < 2 || len(req.Name) > 200 {
		f["name"] = "Nama 2 sampai 200 karakter."
	}
	if len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	ctx := c.Request.Context()
	unusable, _ := platform.RandomToken()
	hash, err := auth.HashPassword(ctx, unusable)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	var u sqlc.User
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		u, err = q.CreateUser(ctx, sqlc.CreateUserParams{OrgID: h.OrgID, Kind: "staff", Email: email, DisplayName: req.Name, PasswordHash: hash})
		if platform.IsUniqueViolation(err, "") {
			return platform.ErrValidation(map[string]string{"email": "Email sudah dipakai akun lain."})
		}
		if err != nil {
			return err
		}
		if _, err := platform.Enqueue(ctx, q, h.OrgID, auth.TopicPasswordReset, "user", u.ID.String(), gin.H{"userId": u.ID}); err != nil {
			return err
		}
		return h.audit(ctx, q, c, "admin.user_create", "user", u.ID.String(), "", nil)
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": u.ID, "email": u.Email})
}

type assignReq struct {
	RoleKey  string     `json:"roleKey"`
	UnitCode string     `json:"unitCode"`
	JobID    *uuid.UUID `json:"jobId"`
	Reason   string     `json:"reason"`
}

// POST /admin/users/:id/assignments
func (h *Handler) Assign(c *gin.Context) {
	uid, ok := pathID(c)
	if !ok {
		return
	}
	p := auth.Current(c)
	var req assignReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	role, err := h.Q.GetRoleByKey(ctx, sqlc.GetRoleByKeyParams{OrgID: h.OrgID, Key: req.RoleKey})
	if platform.IsNoRows(err) {
		platform.Fail(c, platform.ErrValidation(map[string]string{"roleKey": "Peran tidak dikenal."}))
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	// Memberi peran super admin hanya oleh pemegang roles.manage, dan selalu berlaku se-organisasi.
	if role.Key == "super_admin" && (!p.CanAny(auth.PermRolesManage) || req.UnitCode != "" || req.JobID != nil) {
		platform.Fail(c, platform.ErrForbidden())
		return
	}
	var unitID *uuid.UUID
	if req.UnitCode != "" {
		units, err := h.Q.ListUnits(ctx, h.OrgID)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		for _, u := range units {
			if u.Code == req.UnitCode && u.Active {
				unitID = &u.ID
			}
		}
		if unitID == nil {
			platform.Fail(c, platform.ErrValidation(map[string]string{"unitCode": "Unit tidak dikenal atau nonaktif."}))
			return
		}
	}
	if unitID != nil && req.JobID != nil {
		platform.Fail(c, platform.ErrValidation(map[string]string{"jobId": "Pilih unit atau lowongan, tidak keduanya."}))
		return
	}
	if req.JobID != nil {
		// job_id tidak ber-FK di tabel penugasan, jadi keberadaan dan organisasinya diperiksa di sini.
		job, err := h.Q.GetJob(ctx, *req.JobID)
		if platform.IsNoRows(err) || (err == nil && job.OrgID != h.OrgID) {
			platform.Fail(c, platform.ErrValidation(map[string]string{"jobId": "Lowongan tidak dikenal."}))
			return
		}
		if err != nil {
			platform.Fail(c, err)
			return
		}
	}
	target, err := h.Q.GetUser(ctx, uid)
	if platform.IsNoRows(err) || (err == nil && (target.OrgID != h.OrgID || target.Kind != "staff")) {
		platform.Fail(c, platform.ErrNotFound("Pengguna tidak ditemukan."))
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	var id uuid.UUID
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		id, err = q.InsertAssignment(ctx, sqlc.InsertAssignmentParams{OrgID: h.OrgID, UserID: uid, RoleID: role.ID, UnitID: unitID, JobID: req.JobID, GrantedBy: &p.UserID})
		if platform.IsUniqueViolation(err, "") {
			return platform.ErrConflict("already_assigned", "Peran dengan scope yang sama sudah aktif.")
		}
		if err != nil {
			return err
		}
		return h.audit(ctx, q, c, "admin.role_grant", "user", uid.String(), req.Reason, map[string]any{"role": role.Key, "unit": req.UnitCode, "job": req.JobID})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// ensureNotLastAdmin menolak perubahan yang menghilangkan super admin aktif terakhir.
// Baris super_admin aktif dikunci sehingga dua pencabutan bersamaan tidak sama-sama lolos.
func ensureNotLastAdmin(ctx context.Context, q *sqlc.Queries, org uuid.UUID, removing func(assignmentID, userID uuid.UUID) bool) error {
	rows, err := q.LockActiveSuperAdmins(ctx, org)
	if err != nil {
		return err
	}
	remaining := map[uuid.UUID]bool{}
	for _, r := range rows {
		if !removing(r.ID, r.UserID) {
			remaining[r.UserID] = true
		}
	}
	if len(rows) > 0 && len(remaining) == 0 {
		return platform.ErrConflict("last_admin", "Tidak dapat menghapus Super Admin aktif terakhir.")
	}
	return nil
}

// POST /admin/assignments/:id/revoke. Berlaku pada permintaan berikutnya (hak akses dibaca ulang tiap request).
func (h *Handler) Revoke(c *gin.Context) {
	aid, ok := pathID(c)
	if !ok {
		return
	}
	p := auth.Current(c)
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	ctx := c.Request.Context()
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		a, err := q.LockAssignment(ctx, aid)
		if platform.IsNoRows(err) || (err == nil && (a.OrgID != h.OrgID || a.RevokedAt != nil)) {
			return platform.ErrNotFound("Penugasan tidak ditemukan.")
		}
		if err != nil {
			return err
		}
		if a.RoleKey == "super_admin" {
			if !p.CanAny(auth.PermRolesManage) {
				return platform.ErrForbidden()
			}
			if err := ensureNotLastAdmin(ctx, q, h.OrgID, func(id, _ uuid.UUID) bool { return id == aid }); err != nil {
				return err
			}
		}
		if err := q.RevokeAssignment(ctx, sqlc.RevokeAssignmentParams{ID: aid, RevokedBy: &p.UserID}); err != nil {
			return err
		}
		return h.audit(ctx, q, c, "admin.role_revoke", "user", a.UserID.String(), req.Reason, map[string]any{"role": a.RoleKey, "assignment": aid})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /admin/users/:id/status {status: active|disabled}. Menonaktifkan juga mencabut semua sesi.
func (h *Handler) SetStatus(c *gin.Context) {
	uid, ok := pathID(c)
	if !ok {
		return
	}
	p := auth.Current(c)
	var req struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if req.Status != "active" && req.Status != "disabled" {
		platform.Fail(c, platform.ErrValidation(map[string]string{"status": "Status tidak dikenal."}))
		return
	}
	if uid == p.UserID && req.Status == "disabled" {
		platform.Fail(c, platform.ErrConflict("self_disable", "Anda tidak dapat menonaktifkan akun sendiri."))
		return
	}
	ctx := c.Request.Context()
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		u, err := q.GetUser(ctx, uid)
		if platform.IsNoRows(err) || (err == nil && (u.OrgID != h.OrgID || u.Kind != "staff")) {
			return platform.ErrNotFound("Pengguna tidak ditemukan.")
		}
		if err != nil {
			return err
		}
		if req.Status == "disabled" {
			if err := ensureNotLastAdmin(ctx, q, h.OrgID, func(_, user uuid.UUID) bool { return user == uid }); err != nil {
				return err
			}
			if _, err := q.RevokeUserSessions(ctx, sqlc.RevokeUserSessionsParams{UserID: uid, RevokeReason: strp("user_disabled")}); err != nil {
				return err
			}
		}
		if err := q.SetUserStatus(ctx, sqlc.SetUserStatusParams{ID: uid, Status: req.Status}); err != nil {
			return err
		}
		return h.audit(ctx, q, c, "admin.user_"+req.Status, "user", uid.String(), req.Reason, nil)
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ---- Audit (read-only) ----

// GET /admin/audit?action=&target=&cursor=
func (h *Handler) Audit(c *gin.Context) {
	p := auth.Current(c)
	s := p.Scope(auth.PermAuditRead)
	params := sqlc.ListAuditParams{OrgID: h.OrgID, OrgWide: s.OrgWide, UnitIds: s.Units, PageSize: 51}
	if v := strings.TrimSpace(c.Query("action")); v != "" {
		params.Action = &v
	}
	if v := strings.TrimSpace(c.Query("target")); v != "" {
		params.TargetID = &v
	}
	if v := c.Query("cursor"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			platform.Fail(c, platform.ErrValidation(map[string]string{"cursor": "Halaman tidak valid."}))
			return
		}
		params.CursorID = &n
	}
	rows, err := h.Q.ListAudit(c.Request.Context(), params)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	var next *string
	if len(rows) > 50 {
		s := strconv.FormatInt(rows[49].ID, 10)
		next = &s
		rows = rows[:50]
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "nextCursor": next})
}

// ---- Pengaturan berversi ----

type settingSpec struct {
	Label    string
	Validate func(json.RawMessage) string
}

func intRange(min, max int) func(json.RawMessage) string {
	return func(v json.RawMessage) string {
		var n int
		if json.Unmarshal(v, &n) != nil || n < min || n > max {
			return "Nilai harus bilangan " + strconv.Itoa(min) + " sampai " + strconv.Itoa(max) + "."
		}
		return ""
	}
}

// Allowlist pengaturan. Nilai awal yang belum dikonfirmasi pelanggan dicatat di docs/product/assumptions.md.
var Specs = map[string]settingSpec{
	"retention.candidate_months":          {"Retensi data pelamar (bulan)", intRange(1, 120)},
	"security.session_idle_staff_minutes": {"Batas idle sesi staf (menit)", intRange(5, 240)},
	"exam.default_max_attempts":           {"Kesempatan ujian bawaan", intRange(1, 5)},
	"exam.default_extra_minutes_cap":      {"Batas akomodasi waktu (menit)", intRange(0, 240)},
}

// GET /admin/settings
func (h *Handler) ListSettings(c *gin.Context) {
	rows, err := h.Q.ListSettings(c.Request.Context(), h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	byKey := map[string]sqlc.SystemSetting{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	type item struct {
		Key         string          `json:"key"`
		Label       string          `json:"label"`
		Value       json.RawMessage `json:"value"`
		Version     int32           `json:"version"`
		EffectiveAt *time.Time      `json:"effectiveAt"`
	}
	out := []item{}
	keys := make([]string, 0, len(Specs))
	for k := range Specs {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		it := item{Key: k, Label: Specs[k].Label, Value: json.RawMessage("null")}
		if r, ok := byKey[k]; ok {
			it.Value, it.Version, it.EffectiveAt = r.Value, r.Version, &r.EffectiveAt
		}
		out = append(out, it)
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

type settingReq struct {
	Value           json.RawMessage `json:"value"`
	ExpectedVersion int32           `json:"expectedVersion"`
	EffectiveAt     *time.Time      `json:"effectiveAt"`
	Reason          string          `json:"reason"`
}

// PUT /admin/settings/:key: kontrol versi (expectedVersion), waktu efektif, riwayat, dan audit.
func (h *Handler) PutSetting(c *gin.Context) {
	key := c.Param("key")
	spec, ok := Specs[key]
	if !ok {
		platform.Fail(c, platform.ErrNotFound("Pengaturan tidak dikenal."))
		return
	}
	p := auth.Current(c)
	var req settingReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if msg := spec.Validate(req.Value); msg != "" {
		platform.Fail(c, platform.ErrValidation(map[string]string{"value": msg}))
		return
	}
	if len(strings.TrimSpace(req.Reason)) < 5 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"reason": "Alasan perubahan wajib diisi."}))
		return
	}
	eff := time.Now()
	if req.EffectiveAt != nil {
		eff = *req.EffectiveAt
	}
	ctx := c.Request.Context()
	var out sqlc.SystemSetting
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		out, err = q.UpsertSetting(ctx, sqlc.UpsertSettingParams{OrgID: h.OrgID, Key: key, Value: req.Value, EffectiveAt: eff, UpdatedBy: &p.UserID, ExpectedVersion: req.ExpectedVersion})
		if platform.IsNoRows(err) {
			return platform.ErrConflict("stale_version", "Pengaturan sudah diubah orang lain. Muat ulang.")
		}
		if err != nil {
			return err
		}
		if err := q.InsertSettingHistory(ctx, sqlc.InsertSettingHistoryParams{OrgID: h.OrgID, Key: key, Version: out.Version, Value: req.Value,
			EffectiveAt: eff, ChangedBy: &p.UserID, Reason: req.Reason}); err != nil {
			return err
		}
		return h.audit(ctx, q, c, "settings.update", "setting", key, req.Reason, map[string]any{"version": out.Version, "effective_at": eff})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// GET /admin/settings/:key/history
func (h *Handler) SettingHistory(c *gin.Context) {
	rows, err := h.Q.ListSettingHistory(c.Request.Context(), sqlc.ListSettingHistoryParams{OrgID: h.OrgID, Key: c.Param("key")})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

// ---- Organisasi, brand, master data ----

// GET /admin/organization
func (h *Handler) GetOrganization(c *gin.Context) {
	ctx := c.Request.Context()
	o, err := h.Q.GetOrganization(ctx, h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	units, _ := h.Q.ListUnits(ctx, h.OrgID)
	locs, _ := h.Q.ListLocations(ctx, h.OrgID)
	c.JSON(http.StatusOK, gin.H{"organization": o, "units": units, "locations": locs})
}

type orgReq struct {
	Name    string `json:"name"`
	Version int32  `json:"version"`
	Profile struct {
		Tagline string `json:"tagline"`
		About   string `json:"about"`
	} `json:"profile"`
	Brand struct {
		DisplayName  string `json:"displayName"`
		PrimaryColor string `json:"primaryColor"`
	} `json:"brand"`
}

// PUT /admin/organization: perubahan branding langsung terlihat di portal (cache publik diinvalidasi).
func (h *Handler) PutOrganization(c *gin.Context) {
	var req orgReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	f := map[string]string{}
	if l := len(strings.TrimSpace(req.Name)); l < 2 || l > 200 {
		f["name"] = "Nama organisasi 2 sampai 200 karakter."
	}
	if len(req.Profile.Tagline) > 300 || len(req.Profile.About) > 3000 {
		f["profile"] = "Profil terlalu panjang."
	}
	if l := len(strings.TrimSpace(req.Brand.DisplayName)); l < 2 || l > 80 {
		f["brand.displayName"] = "Nama tampilan 2 sampai 80 karakter."
	}
	if !hexColor(req.Brand.PrimaryColor) {
		f["brand.primaryColor"] = "Warna harus format #RRGGBB."
	}
	if len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	ctx := c.Request.Context()
	prof, _ := json.Marshal(req.Profile)
	brand, _ := json.Marshal(req.Brand)
	var out sqlc.Organization
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		out, err = q.UpdateOrganization(ctx, sqlc.UpdateOrganizationParams{ID: h.OrgID, Version: req.Version, Name: strings.TrimSpace(req.Name), Profile: prof, Brand: brand})
		if platform.IsNoRows(err) {
			return platform.ErrConflict("stale_version", "Profil organisasi sudah diubah orang lain. Muat ulang.")
		}
		if err != nil {
			return err
		}
		return h.audit(ctx, q, c, "settings.organization_update", "organization", h.OrgID.String(), "", map[string]any{"version": out.Version})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if h.InvalidatePublic != nil {
		h.InvalidatePublic(ctx)
	}
	c.JSON(http.StatusOK, out)
}

func hexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

// POST /admin/units/:id/active dan /admin/locations/:id/active: menonaktifkan tanpa menghapus histori.
func (h *Handler) SetMasterActive(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := pathID(c)
		if !ok {
			return
		}
		var req struct {
			Active bool `json:"active"`
		}
		if err := platform.BindJSON(c, &req); err != nil {
			platform.Fail(c, err)
			return
		}
		ctx := c.Request.Context()
		err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
			q := h.Q.WithTx(tx)
			var n int64
			var err error
			if kind == "unit" {
				n, err = q.SetUnitActive(ctx, sqlc.SetUnitActiveParams{ID: id, OrgID: h.OrgID, Active: req.Active})
			} else {
				n, err = q.SetLocationActive(ctx, sqlc.SetLocationActiveParams{ID: id, OrgID: h.OrgID, Active: req.Active})
			}
			if err != nil {
				return err
			}
			if n == 0 {
				return platform.ErrNotFound("")
			}
			return h.audit(ctx, q, c, "settings."+kind+"_active", kind, id.String(), "", map[string]any{"active": req.Active})
		})
		if err != nil {
			platform.Fail(c, err)
			return
		}
		if h.InvalidatePublic != nil {
			h.InvalidatePublic(ctx)
		}
		c.Status(http.StatusNoContent)
	}
}

// ---- Komunikasi dan antrean ----

// GET /admin/deliveries
func (h *Handler) Deliveries(c *gin.Context) {
	rows, err := h.Q.ListDeliveries(c.Request.Context())
	if err != nil {
		platform.Fail(c, err)
		return
	}
	dead, err := h.Q.ListDeadOutbox(c.Request.Context(), h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	st, err := h.Q.OutboxStats(c.Request.Context())
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deliveries": rows, "dead": dead, "outbox": st})
}

// POST /admin/outbox/:id/replay: dead-letter dikembalikan ke antrean dengan audit. Consumer idempoten.
func (h *Handler) Replay(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		n, err := q.ReplayDeadOutbox(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return platform.ErrNotFound("Event dead-letter tidak ditemukan.")
		}
		return h.audit(ctx, q, c, "ops.outbox_replay", "outbox", id.String(), "", nil)
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func strp(s string) *string { return &s }
