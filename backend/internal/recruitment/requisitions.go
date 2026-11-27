package recruitment

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Kode unit memakai format CHECK di tabel units: huruf besar, angka, atau tanda hubung, 2 sampai 20 karakter.
var unitCodeRe = regexp.MustCompile(`^[A-Z0-9-]{2,20}$`)

// unitKodeAda memeriksa kode unit ada di organisasi, aktif atau nonaktif, agar riwayat requisition tetap bisa difilter.
func (h *Handler) unitKodeAda(c *gin.Context, kode string) (bool, error) {
	units, err := h.Q.ListUnits(c.Request.Context(), h.OrgID)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(units, func(u sqlc.Unit) bool { return u.Code == kode }), nil
}

// GET /requisitions: scope unit dari requisitions.read ditambah requisition milik sendiri. Filter unitCode (opsional)
// dicocokkan di server lewat ID unit dan hanya mempersempit hasil; scope tetap berlaku.
func (h *Handler) ListRequisitions(c *gin.Context) {
	p := auth.Current(c)
	s := p.Scope(auth.PermRequisitionsRead)
	unitCode := optStr(c, "unitCode")
	if unitCode != nil {
		if !unitCodeRe.MatchString(*unitCode) {
			platform.Fail(c, platform.ErrValidation(map[string]string{"unitCode": "Kode unit terdiri dari huruf besar, angka, atau tanda hubung, 2 sampai 20 karakter."}))
			return
		}
		ada, err := h.unitKodeAda(c, *unitCode)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		if !ada {
			platform.Fail(c, platform.ErrValidation(map[string]string{"unitCode": "Kode unit tidak dikenal."}))
			return
		}
	}
	rows, err := h.Q.ListRequisitions(c.Request.Context(), sqlc.ListRequisitionsParams{OrgID: h.OrgID, OrgWide: s.OrgWide, UnitIds: s.Units, UserID: p.UserID, UnitCode: unitCode})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

// Track rekrutmen (BRD Bab 3.1): nilai tetap untuk requisition, lowongan, dan filter daftar.
var tracks = []string{"OPERATOR", "STAFF", "MT_S1_D4"}

// Alasan permintaan tenaga kerja (BRD Bab 3.2).
var reasonCategories = []string{"penambahan", "penggantian", "proyek_baru"}

type requisitionReq struct {
	UnitCode       string  `json:"unitCode"`
	LocationCode   string  `json:"locationCode"`
	Title          string  `json:"title"`
	Headcount      int32   `json:"headcount"`
	EmploymentType string  `json:"employmentType"`
	Justification  string  `json:"justification"`
	Track          string  `json:"track"`
	Line           string  `json:"line"`
	TargetJoinDate *string `json:"targetJoinDate"`
	ReasonCategory string  `json:"reasonCategory"`
}

// POST /requisitions: permintaan tenaga kerja internal (belum publikasi). Pembuat menjadi hiring manager bila perannya HM.
func (h *Handler) CreateRequisition(c *gin.Context) {
	p := auth.Current(c)
	var req requisitionReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	f := map[string]string{}
	if l := len(strings.TrimSpace(req.Title)); l < 3 || l > 200 {
		f["title"] = "Judul 3 sampai 200 karakter."
	}
	if req.Headcount < 1 || req.Headcount > 1000 {
		f["headcount"] = "Jumlah kebutuhan 1 sampai 1000."
	}
	if !slices.Contains(employmentTypes, req.EmploymentType) {
		f["employmentType"] = "Pilih jenis kerja."
	}
	if len(req.Justification) > 4000 {
		f["justification"] = "Alasan maksimal 4000 karakter."
	}
	if !slices.Contains(tracks, req.Track) {
		f["track"] = "Pilih track: OPERATOR, STAFF, atau MT_S1_D4."
	}
	if !slices.Contains(reasonCategories, req.ReasonCategory) {
		f["reasonCategory"] = "Pilih alasan: penambahan, penggantian, atau proyek_baru."
	}
	lineBersih := strings.TrimSpace(req.Line)
	if utf8.RuneCountInString(lineBersih) > 100 {
		f["line"] = "Line maksimal 100 karakter."
	}
	var line *string
	if lineBersih != "" {
		line = &lineBersih
	}
	var target *time.Time
	if req.TargetJoinDate != nil && strings.TrimSpace(*req.TargetJoinDate) != "" {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(*req.TargetJoinDate))
		if err != nil {
			f["targetJoinDate"] = "Tanggal target masuk memakai format YYYY-MM-DD."
		} else {
			target = &t
		}
	}
	unit, err := h.unitByCode(c, req.UnitCode)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if unit == nil {
		f["unitCode"] = "Pilih unit."
	}
	loc, err := h.locationByCode(c, req.LocationCode)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if loc == nil {
		f["locationCode"] = "Pilih lokasi."
	}
	if len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	var hm *uuid.UUID
	if slices.Contains(p.Roles(), "hiring_manager") {
		hm = &p.UserID
	}
	ctx := c.Request.Context()
	var r sqlc.Requisition
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		r, err = q.CreateRequisition(ctx, sqlc.CreateRequisitionParams{OrgID: h.OrgID, UnitID: unit.ID, LocationID: loc.ID, Title: strings.TrimSpace(req.Title),
			Headcount: req.Headcount, EmploymentType: req.EmploymentType, Justification: req.Justification, RequestedBy: p.UserID, HiringManagerID: hm,
			Track: req.Track, Line: line, TargetJoinDate: target, ReasonCategory: &req.ReasonCategory})
		if err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "requisition.create",
			TargetType: "requisition", TargetID: r.ID.String(), ScopeUnit: &unit.ID, RequestID: platform.RequestID(c),
			Detail: map[string]any{"track": req.Track, "headcount": req.Headcount, "reasonCategory": req.ReasonCategory}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, r)
}

// hariIniOrganisasi: tanggal kalender hari ini di zona waktu organisasi, dinyatakan pukul 00:00 UTC seperti kolom date.
func hariIniOrganisasi(ctx context.Context, q *sqlc.Queries, orgID uuid.UUID) (time.Time, error) {
	org, err := q.GetOrganization(ctx, orgID)
	if err != nil {
		return time.Time{}, err
	}
	loc, err := time.LoadLocation(org.Timezone)
	if err != nil {
		return time.Time{}, err
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
}

// RequisitionAction: submit (pemohon), approve/reject (requisitions.approve di unit, bukan pemohon sendiri), cancel (pemohon).
func (h *Handler) RequisitionAction(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := auth.Current(c)
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			platform.Fail(c, platform.ErrNotFound(""))
			return
		}
		var req struct {
			Version int32  `json:"version"`
			Note    string `json:"note"`
		}
		if err := platform.BindJSON(c, &req); err != nil {
			platform.Fail(c, err)
			return
		}
		ctx := c.Request.Context()
		var out sqlc.Requisition
		err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
			q := h.Q.WithTx(tx)
			r, err := q.LockRequisition(ctx, id)
			if platform.IsNoRows(err) || (err == nil && r.OrgID != h.OrgID) {
				return platform.ErrNotFound("Permintaan tidak ditemukan.")
			}
			if err != nil {
				return err
			}
			own := r.RequestedBy == p.UserID || (r.HiringManagerID != nil && *r.HiringManagerID == p.UserID)
			if !own && !p.Can(auth.PermRequisitionsRead, auth.Resource{Unit: r.UnitID}) {
				return platform.ErrNotFound("Permintaan tidak ditemukan.")
			}
			from := map[string][]string{"submit": {"draft"}, "approve": {"submitted"}, "reject": {"submitted"}, "cancel": {"draft", "submitted"}}[action]
			to := map[string]string{"submit": "submitted", "approve": "approved", "reject": "rejected", "cancel": "cancelled"}[action]
			if !slices.Contains(from, r.Status) {
				return platform.ErrConflict("invalid_status", "Status permintaan tidak memungkinkan tindakan ini.")
			}
			switch action {
			case "submit", "cancel":
				if r.RequestedBy != p.UserID {
					return platform.ErrForbidden()
				}
			case "approve", "reject":
				if !p.Can(auth.PermRequisitionsApprove, auth.Resource{Unit: r.UnitID}) {
					return platform.ErrForbidden()
				}
				if r.RequestedBy == p.UserID {
					return platform.ErrConflict("self_approval", "Anda tidak dapat menyetujui permintaan Anda sendiri.")
				}
				if action == "reject" && len(strings.TrimSpace(req.Note)) < 5 {
					return platform.ErrValidation(map[string]string{"note": "Alasan penolakan wajib diisi."})
				}
			}
			if action == "submit" && r.TargetJoinDate != nil {
				hari, err := hariIniOrganisasi(ctx, q, h.OrgID)
				if err != nil {
					return err
				}
				if r.TargetJoinDate.Before(hari) {
					return platform.ErrValidation(map[string]string{"targetJoinDate": "Tanggal target masuk tidak boleh di masa lalu."})
				}
			}
			out, err = q.SetRequisitionStatus(ctx, sqlc.SetRequisitionStatusParams{ID: id, Status: to, DecidedBy: &p.UserID, Note: req.Note, ExpectedVersion: req.Version})
			if platform.IsNoRows(err) {
				return platform.ErrConflict("stale_version", "Permintaan sudah diubah orang lain. Muat ulang.")
			}
			if err != nil {
				return err
			}
			return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "requisition." + action,
				TargetType: "requisition", TargetID: id.String(), ScopeUnit: &r.UnitID, Reason: req.Note, RequestID: platform.RequestID(c)})
		})
		if err != nil {
			platform.Fail(c, err)
			return
		}
		c.JSON(http.StatusOK, out)
	}
}
