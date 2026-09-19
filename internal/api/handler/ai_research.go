// AI contact research endpoints. A run researches one contact on the public web
// and saves cited findings; it charges 2 credits per run (billable even when it
// finds nothing). Sync runs execute in the request; the batch endpoint queues
// and drains in the background. Gated by the AI_RESEARCH scope (JWT callers by
// the matching contact permission).
package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/api/middleware"
	"github.com/warmbly/warmbly/internal/app/aitools"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// aiActorInvocation builds a minimal invocation for AI feature endpoints. The
// route middleware already gated access; the research tools (search_web,
// fetch_url) require no permission, so only identity is needed here.
func (h *Handler) aiActorInvocation(c *gin.Context) (aitools.Invocation, *errx.Error) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		return aitools.Invocation{}, errx.New(errx.BadRequest, "no organization selected")
	}
	userID, _ := middleware.GetUserUUID(c) // uuid.Nil for API-key callers
	return aitools.Invocation{
		OrgID:     *orgID,
		UserID:    userID,
		IsAPIKey:  middleware.GetAPIKeyPermissions(c) != 0,
		APIPerms:  middleware.GetAPIKeyPermissions(c),
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}, nil
}

// ResearchContact — POST /contacts/:id/research (sync)
func (h *Handler) ResearchContact(c *gin.Context) {
	if h.ResearchService == nil {
		errx.JSON(c, errx.New(errx.ServiceUnavailable, "AI research is not configured"))
		return
	}
	inv, xerr := h.aiActorInvocation(c)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	contactID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid contact id"))
		return
	}
	var req struct {
		Objective string `json:"objective"`
	}
	_ = c.ShouldBindJSON(&req)

	idemKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	run, rerr := h.ResearchService.RunResearch(c.Request.Context(), inv, contactID, req.Objective, idemKey)
	if rerr != nil {
		errx.JSON(c, rerr)
		return
	}
	c.JSON(http.StatusOK, run)
}

// ListContactResearch — GET /contacts/:id/research
func (h *Handler) ListContactResearch(c *gin.Context) {
	if h.ResearchService == nil {
		errx.JSON(c, errx.New(errx.ServiceUnavailable, "AI research is not configured"))
		return
	}
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	contactID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid contact id"))
		return
	}
	limit, xerr := parseCreditLimit(c.Query("limit"), 20, 100)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	runs, rerr := h.ResearchService.ListRuns(c.Request.Context(), *orgID, contactID, limit)
	if rerr != nil {
		errx.JSON(c, rerr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": runs})
}

// BatchResearch — POST /contacts/research/batch
func (h *Handler) BatchResearch(c *gin.Context) {
	if h.ResearchService == nil {
		errx.JSON(c, errx.New(errx.ServiceUnavailable, "AI research is not configured"))
		return
	}
	inv, xerr := h.aiActorInvocation(c)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	var req struct {
		models.ContactSelection
		// contact_ids is the original field name; the selection's `contacts`,
		// `all` and `filters` are the "select all matching" form.
		ContactIDs []uuid.UUID `json:"contact_ids"`
		Objective  string      `json:"objective"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid request body"))
		return
	}
	sel := req.ContactSelection
	if !sel.All && len(sel.Contacts) == 0 {
		for _, id := range req.ContactIDs {
			sel.Contacts = append(sel.Contacts, id.String())
		}
	}
	resolved, ok := h.resolveContactSelection(c, inv.OrgID, sel)
	if !ok {
		return
	}
	contactIDs := make([]uuid.UUID, 0, len(resolved))
	for _, raw := range resolved {
		id, perr := uuid.Parse(raw)
		if perr != nil {
			errx.JSON(c, errx.ErrUuid)
			return
		}
		contactIDs = append(contactIDs, id)
	}

	queued, rerr := h.ResearchService.Batch(c.Request.Context(), inv, contactIDs, req.Objective)
	if rerr != nil {
		errx.JSON(c, rerr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"queued": queued})
}
