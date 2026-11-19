package notify

import (
	"net/http"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type InboxHandler struct{ Q *sqlc.Queries }

// GET /me/notifications: hanya milik user yang login.
func (h *InboxHandler) List(c *gin.Context) {
	rows, err := h.Q.ListNotifications(c.Request.Context(), auth.Current(c).UserID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

// POST /me/notifications/:id/read
func (h *InboxHandler) MarkRead(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound(""))
		return
	}
	n, err := h.Q.MarkNotificationRead(c.Request.Context(), sqlc.MarkNotificationReadParams{ID: id, UserID: auth.Current(c).UserID})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if n == 0 {
		platform.Fail(c, platform.ErrNotFound("Notifikasi tidak ditemukan."))
		return
	}
	c.Status(http.StatusNoContent)
}
