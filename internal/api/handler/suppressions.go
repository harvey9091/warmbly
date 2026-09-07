package handler

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/api/middleware"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// The workspace suppression list: every address and domain campaign mail
// must not go to, whatever put it there.

func (h *Handler) ListSuppressions(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		l, err := strconv.Atoi(raw)
		if err != nil || l < 1 || l > 200 {
			errx.JSON(c, errx.New(errx.BadRequest, "limit must be between 1 and 200"))
			return
		}
		limit = l
	}
	var beforeAt *time.Time
	var beforeID *uuid.UUID
	if raw := c.Query("cursor"); raw != "" {
		at, id, ok := decodeSuppressionCursor(raw)
		if !ok {
			errx.JSON(c, errx.New(errx.BadRequest, "invalid cursor"))
			return
		}
		beforeAt, beforeID = &at, &id
	}

	rows, xerr := h.AdvancedService.ListSuppressions(c.Request.Context(), *orgID, c.Query("q"), beforeAt, beforeID, limit+1)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	res := models.SuppressionListResult{Data: rows}
	if len(rows) > limit {
		res.Data = rows[:limit]
		last := res.Data[limit-1]
		next := encodeSuppressionCursor(last.CreatedAt, last.ID)
		res.Pagination = models.CPagination{NextCursor: &next, HasMore: true}
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) AddSuppressions(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	actorID, _ := middleware.GetUserUUID(c)
	var req models.AddSuppressionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	res, xerr := h.AdvancedService.AddSuppressions(c.Request.Context(), *orgID, actorID, &req)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionCreate, models.AuditEntitySuppression, nil, nil, map[string]string{
		"added":   strconv.Itoa(res.Added),
		"skipped": strconv.Itoa(len(res.Skipped)),
	})
	c.JSON(http.StatusOK, res)
}

func (h *Handler) RemoveSuppression(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid suppression id"))
		return
	}
	entry, xerr := h.AdvancedService.RemoveSuppression(c.Request.Context(), *orgID, id)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	// Lifting a recipient's own opt-out is the audited action: the entry it
	// removed is recorded so the trail shows who was re-enabled and why they
	// had been on the list.
	h.auditOrg(c, models.AuditActionDelete, models.AuditEntitySuppression, &id, nil, map[string]string{
		"value":  entry.Email,
		"kind":   string(entry.Kind),
		"source": string(entry.Source),
	})
	c.Status(http.StatusNoContent)
}

// The cursor is the keyset (created_at, id) of the last row, base64 so
// clients treat it as opaque.
func encodeSuppressionCursor(at time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeSuppressionCursor(raw string) (time.Time, uuid.UUID, bool) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, uuid.Nil, false
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, false
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, false
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, false
	}
	return at, id, true
}
