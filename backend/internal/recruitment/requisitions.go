package recruitment

import (
	"net/http"
	"slices"
	"strings"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// GET /requisitions: scope unit dari requisitions.read ditambah requisition milik sendiri.
func (h *Handler) ListRequisitions(c *gin.Context) {
	p := auth.Current(c)
	s := p.Scope(auth.PermRequisitionsRead)
	rows, err := h.Q.ListRequisitions(c.Request.Context(), sqlc.ListRequisitionsParams{OrgID: h.OrgID, OrgWide: s.OrgWide, UnitIds: s.Units, UserID: p.UserID})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

type requisitionReq struct {
	UnitCode       string `json:"unitCode"`
	LocationCode   string `json:"locationCode"`
	Title          string `json:"title"`
	Headcount      int32  `json:"headcount"`
	EmploymentType string `json:"employmentType"`
	Justification  string `json:"justification"`
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
			Headcount: req.Headcount, EmploymentType: req.EmploymentType, Justification: req.Justification, RequestedBy: p.UserID, HiringManagerID: hm})
		if err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "requisition.create",
			TargetType: "requisition", TargetID: r.ID.String(), ScopeUnit: &unit.ID, RequestID: platform.RequestID(c)})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, r)
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
