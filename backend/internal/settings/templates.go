package settings

import (
	"net/http"
	"slices"
	"sort"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/notify"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type templateDTO struct {
	Key          string            `json:"key"`
	Channel      string            `json:"channel"`
	Label        string            `json:"label"`
	When         string            `json:"when"`
	Subject      string            `json:"subject"`
	Body         string            `json:"body"`
	Version      int32             `json:"version"`    // 0 = belum pernah diubah (bawaan kode)
	Customized   bool              `json:"customized"` // false = isi sama dengan bawaan
	Required     []string          `json:"required"`
	Placeholders map[string]string `json:"placeholders"`
}

func channels(key string) []string {
	out := []string{"email"}
	if _, ok := notify.WhatsAppTemplates[key]; ok {
		out = append(out, "whatsapp")
	}
	return out
}

func validChannel(key, ch string) bool { return slices.Contains(channels(key), ch) }

// GET /admin/message-templates
func (h *Handler) ListTemplates(c *gin.Context) {
	rows, err := h.Q.LatestMessageTemplates(c.Request.Context(), h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	latest := map[string]sqlc.MessageTemplate{}
	for _, r := range rows {
		latest[r.Key+"/"+r.Channel] = r
	}
	keys := make([]string, 0, len(notify.TemplateCatalog))
	for k := range notify.TemplateCatalog {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return notify.TemplateCatalog[keys[i]].Label < notify.TemplateCatalog[keys[j]].Label
	})
	out := []templateDTO{}
	for _, k := range keys {
		for _, ch := range channels(k) {
			def, _ := notify.Default(k, ch)
			d := templateDTO{Key: k, Channel: ch, Label: notify.TemplateCatalog[k].Label, When: notify.TemplateCatalog[k].When,
				Subject: def.Subject, Body: def.Body, Required: notify.Required(k), Placeholders: notify.PlaceholderLabels}
			if r, ok := latest[k+"/"+ch]; ok {
				d.Subject, d.Body, d.Version, d.Customized = r.Subject, r.Body, r.Version, !r.IsDefault
			}
			out = append(out, d)
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// GET /admin/message-templates/:key/:channel/versions
func (h *Handler) TemplateVersions(c *gin.Context) {
	key, ch := c.Param("key"), c.Param("channel")
	if _, ok := notify.TemplateCatalog[key]; !ok || !validChannel(key, ch) {
		platform.Fail(c, platform.ErrNotFound("Template tidak ditemukan."))
		return
	}
	rows, err := h.Q.ListMessageTemplateVersions(c.Request.Context(), sqlc.ListMessageTemplateVersionsParams{OrgID: h.OrgID, Key: key, Channel: ch})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

type templateReq struct {
	Subject         string `json:"subject"`
	Body            string `json:"body"`
	Note            string `json:"note"`
	ExpectedVersion int32  `json:"expectedVersion"`
	ResetToDefault  bool   `json:"resetToDefault"`
	RestoreVersion  *int32 `json:"restoreVersion"`
}

// PUT /admin/message-templates/:key/:channel: simpan versi baru (isi baru, salinan versi lama, atau bawaan).
func (h *Handler) SaveTemplate(c *gin.Context) {
	key, ch := c.Param("key"), c.Param("channel")
	if _, ok := notify.TemplateCatalog[key]; !ok || !validChannel(key, ch) {
		platform.Fail(c, platform.ErrNotFound("Template tidak ditemukan."))
		return
	}
	var req templateReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	isDefault := false
	switch {
	case req.ResetToDefault:
		def, _ := notify.Default(key, ch)
		req.Subject, req.Body, isDefault = def.Subject, def.Body, true
	case req.RestoreVersion != nil:
		old, err := h.Q.GetMessageTemplateVersion(ctx, sqlc.GetMessageTemplateVersionParams{OrgID: h.OrgID, Key: key, Channel: ch, Version: *req.RestoreVersion})
		if platform.IsNoRows(err) {
			platform.Fail(c, platform.ErrValidation(map[string]string{"restoreVersion": "Versi tidak ditemukan."}))
			return
		}
		if err != nil {
			platform.Fail(c, err)
			return
		}
		req.Subject, req.Body, isDefault = old.Subject, old.Body, old.IsDefault
	}
	if f := notify.ValidateTemplate(key, ch, req.Subject, req.Body); len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	if len(req.Note) > 500 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"note": "Catatan maksimal 500 karakter."}))
		return
	}
	p := auth.Current(c)
	var row sqlc.MessageTemplate
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		cur, err := q.LatestMessageTemplate(ctx, sqlc.LatestMessageTemplateParams{OrgID: h.OrgID, Key: key, Channel: ch})
		var curVersion int32
		if err == nil {
			curVersion = cur.Version
		} else if !platform.IsNoRows(err) {
			return err
		}
		if curVersion != req.ExpectedVersion {
			return platform.ErrConflict("stale_version", "Template sudah diubah orang lain. Muat ulang untuk melihat versi terbaru.")
		}
		row, err = q.InsertMessageTemplate(ctx, sqlc.InsertMessageTemplateParams{OrgID: h.OrgID, Key: key, Channel: ch, Version: curVersion + 1,
			Subject: req.Subject, Body: req.Body, IsDefault: isDefault, Note: req.Note, CreatedBy: &p.UserID})
		if platform.IsUniqueViolation(err, "") {
			return platform.ErrConflict("stale_version", "Template sudah diubah orang lain. Muat ulang untuk melihat versi terbaru.")
		}
		if err != nil {
			return err
		}
		return h.audit(ctx, q, c, "admin.template_save", "message_template", key+"/"+ch, req.Note,
			map[string]any{"version": row.Version, "resetToDefault": req.ResetToDefault, "restoreVersion": req.RestoreVersion})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"version": row.Version})
}

// POST /admin/message-templates/:key/:channel/preview: validasi + hasil render dengan data contoh.
func (h *Handler) PreviewTemplate(c *gin.Context) {
	key, ch := c.Param("key"), c.Param("channel")
	if _, ok := notify.TemplateCatalog[key]; !ok || !validChannel(key, ch) {
		platform.Fail(c, platform.ErrNotFound("Template tidak ditemukan."))
		return
	}
	var req templateReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if f := notify.ValidateTemplate(key, ch, req.Subject, req.Body); len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	subject, body, err := notify.Render(notify.Template{Key: key, Subject: req.Subject, Body: req.Body}, notify.SampleVars)
	if err != nil {
		platform.Fail(c, platform.ErrValidation(map[string]string{"body": err.Error()}))
		return
	}
	c.JSON(http.StatusOK, gin.H{"subject": subject, "body": body})
}
