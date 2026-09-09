package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/api/middleware"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// Small helpers shared by the admin handlers. They used to live in the
// SSH worker file, which no longer exists.

func itoa(n int) string { return strconv.Itoa(n) }

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// parseID reads the :id path parameter as a uuid.
func (h *Handler) parseID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid id"))
		return uuid.Nil, false
	}
	return id, true
}

// audit fires an admin audit log entry. Writes to admin_audit_log (the table
// the /admin/audit-logs viewer queries) so every operator action is browsable
// alongside ban/unban and the rest.
//
// Fire-and-forget: AdminService spawns its own goroutine, so the response is
// never blocked. Safe to call with a nil entityID and/or nil metadata.
func (h *Handler) audit(c *gin.Context, action models.AuditAction, entity models.AuditEntityType, entityID *uuid.UUID, metadata map[string]string) {
	if h.AdminService == nil {
		return
	}
	adminID := middleware.GetAdminUserID(c)
	if adminID == nil {
		return
	}
	var details map[string]any
	if len(metadata) > 0 {
		details = make(map[string]any, len(metadata))
		for k, v := range metadata {
			details[k] = v
		}
	}
	h.AdminService.LogAdminAction(
		c.Request.Context(),
		*adminID,
		string(action),
		string(entity),
		entityID,
		details,
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
}
