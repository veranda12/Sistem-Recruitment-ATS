package notify

import (
	"net/http"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PreferencesHandler: pilihan pemberitahuan kandidat. Persetujuan WhatsApp disimpan dengan waktunya dan dicatat di audit.
type PreferencesHandler struct {
	Q               *sqlc.Queries
	OrgID           uuid.UUID
	WhatsAppEnabled bool // false bila WHATSAPP_MODE=off: pilihan tetap tersimpan tetapi tidak ada pengiriman
}

type preferencesDTO struct {
	WhatsApp          bool       `json:"whatsapp"`
	WhatsAppSince     *time.Time `json:"whatsappSince"`
	Phone             string     `json:"phone"`
	WhatsAppAvailable bool       `json:"whatsappAvailable"`
}

// GET /me/notification-preferences
func (h *PreferencesHandler) Get(c *gin.Context) {
	p := auth.Current(c)
	row, err := h.Q.GetWhatsAppPreference(c.Request.Context(), p.UserID)
	if platform.IsNoRows(err) {
		c.JSON(http.StatusOK, preferencesDTO{WhatsAppAvailable: h.WhatsAppEnabled})
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, preferencesDTO{WhatsApp: row.WhatsappOptInAt != nil, WhatsAppSince: row.WhatsappOptInAt, Phone: row.Phone, WhatsAppAvailable: h.WhatsAppEnabled})
}

// PUT /me/notification-preferences {whatsapp: bool}
func (h *PreferencesHandler) Put(c *gin.Context) {
	p := auth.Current(c)
	var req struct {
		WhatsApp bool `json:"whatsapp"`
	}
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	cur, err := h.Q.GetWhatsAppPreference(ctx, p.UserID)
	if platform.IsNoRows(err) {
		platform.Fail(c, platform.ErrValidation(map[string]string{"whatsapp": "Lengkapi profil Anda terlebih dahulu."}))
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if req.WhatsApp && NormalizeWhatsApp(cur.Phone) == "" {
		platform.Fail(c, platform.ErrValidation(map[string]string{"whatsapp": "Isi nomor telepon yang valid di profil sebelum mengaktifkan WhatsApp."}))
		return
	}
	var at *time.Time
	if req.WhatsApp {
		at = cur.WhatsappOptInAt // persetujuan lama dipertahankan
		if at == nil {
			now := time.Now()
			at = &now
		}
	}
	row, err := h.Q.SetWhatsAppOptIn(ctx, sqlc.SetWhatsAppOptInParams{UserID: p.UserID, WhatsappOptInAt: at})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if (cur.WhatsappOptInAt != nil) != req.WhatsApp {
		action := "candidate.whatsapp_opt_out"
		if req.WhatsApp {
			action = "candidate.whatsapp_opt_in"
		}
		_ = platform.WriteAudit(ctx, h.Q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "candidate", Action: action,
			TargetType: "user", TargetID: p.UserID.String(), RequestID: platform.RequestID(c)})
	}
	c.JSON(http.StatusOK, preferencesDTO{WhatsApp: row.WhatsappOptInAt != nil, WhatsAppSince: row.WhatsappOptInAt, Phone: row.Phone, WhatsAppAvailable: h.WhatsAppEnabled})
}
