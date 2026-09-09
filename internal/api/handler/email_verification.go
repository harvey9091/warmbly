package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/warmbly/warmbly/internal/api/middleware"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// verifyEmailRequest is the optional JSON body for VerifyEmail. The address may
// also be passed as the `email` query param; the body takes precedence.
type verifyEmailRequest struct {
	Email string `json:"email"`
}

// VerifyEmail verifies a single email address on demand through whichever
// verifier the workspace uses (its connected provider, else the built-in
// check) and returns the emailverify.Result. Nothing is stored.
//
// Control-plane only: the SMTP RCPT probe behind the built-in check runs from
// the backend (a non-sending IP), never a worker. See internal/pkg/emailverify.
func (h *Handler) VerifyEmail(c *gin.Context) {
	if _, err := middleware.GetUserUUID(c); err != nil {
		errx.JSON(c, errx.ErrUnauthorized)
		return
	}
	if h.EmailVerifyService == nil {
		errx.JSON(c, errx.InternalError())
		return
	}
	orgID, ok := requireOrgID(c)
	if !ok {
		return
	}

	var req verifyEmailRequest
	// Body is optional; ignore a bind error and fall back to the query param.
	_ = c.ShouldBindJSON(&req)
	email := strings.TrimSpace(req.Email)
	if email == "" {
		email = strings.TrimSpace(c.Query("email"))
	}
	if email == "" {
		errx.JSON(c, errx.ErrEmail)
		return
	}

	res := h.EmailVerifyService.VerifyAddress(c.Request.Context(), orgID, email)
	c.JSON(http.StatusOK, res)
}

// GetContactVerification reports which verifier the workspace uses, its
// remaining credits, and the contacts by verdict.
// GET /contacts/verification
func (h *Handler) GetContactVerification(c *gin.Context) {
	if h.EmailVerifyService == nil {
		errx.JSON(c, errx.InternalError())
		return
	}
	orgID, ok := requireOrgID(c)
	if !ok {
		return
	}
	out, xerr := h.EmailVerifyService.Overview(c.Request.Context(), orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, out)
}

// RequestContactVerification queues a re-check of the listed contacts, or
// records a manual verdict on them.
// POST /contacts/verification
func (h *Handler) RequestContactVerification(c *gin.Context) {
	if h.EmailVerifyService == nil {
		errx.JSON(c, errx.InternalError())
		return
	}
	orgID, ok := requireOrgID(c)
	if !ok {
		return
	}
	var req models.ContactVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.ErrInvalid)
		return
	}
	// A campaign-scoped re-verify carries no ids of its own, so only resolve a
	// selection that actually names contacts.
	if req.All || len(req.Contacts) > 0 {
		ids, ok := h.resolveContactSelection(c, orgID, req.ContactSelection)
		if !ok {
			return
		}
		req.ContactSelection = models.ContactSelection{Contacts: ids}
	}
	resp, xerr := h.EmailVerifyService.Request(c.Request.Context(), orgID, req)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	// The audit spine refreshes every teammate's contact lists; verdicts
	// then land live as the background pass records them.
	h.auditOrg(c, models.AuditActionUpdate, models.AuditEntityContact, nil, nil, map[string]string{
		"verification": req.Action, "count": itoa(resp.Affected),
	})
	c.JSON(http.StatusOK, resp)
}
