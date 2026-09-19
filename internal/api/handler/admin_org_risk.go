package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/api/middleware"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// adminRiskOverrideRequest pins a workspace's posture to an operator's
// decision. reason is required: a posture nobody can explain is one the next
// operator cannot safely lift.
type adminRiskOverrideRequest struct {
	State  string `json:"state" binding:"required"`
	Reason string `json:"reason" binding:"required"`
}

// AdminGetOrgRisk returns the whole risk record, evidence included. This is
// the operator view: the customer endpoint deliberately withholds the signal
// blob, which left nobody able to see WHY a workspace was flagged.
func (h *Handler) AdminGetOrgRisk(c *gin.Context) {
	orgID, ok := adminOrgParam(c)
	if !ok {
		return
	}
	if h.OrgRiskService == nil {
		errx.JSON(c, errx.New(errx.ServiceUnavailable, "risk posture is not available on this instance"))
		return
	}
	risk, xerr := h.OrgRiskService.Get(c.Request.Context(), orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	c.JSON(http.StatusOK, risk)
}

// AdminSetOrgRiskOverride pins the posture. The pin outranks the score and
// survives every later detector write, so a workspace cleared by review is not
// re-suspended by the evidence that is still on file.
func (h *Handler) AdminSetOrgRiskOverride(c *gin.Context) {
	adminID := middleware.GetAdminUserID(c)
	if adminID == nil {
		errx.JSON(c, errx.ErrUnauthorized)
		return
	}
	orgID, ok := adminOrgParam(c)
	if !ok {
		return
	}
	if h.OrgRiskService == nil {
		errx.JSON(c, errx.New(errx.ServiceUnavailable, "risk posture is not available on this instance"))
		return
	}

	var req adminRiskOverrideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "a state and a reason are required"))
		return
	}
	state := models.OrgRiskState(strings.TrimSpace(req.State))
	if !state.Valid() {
		errx.JSON(c, errx.New(errx.BadRequest, "state must be trusted, watch, restricted or suspended"))
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		errx.JSON(c, errx.New(errx.BadRequest, "a reason is required"))
		return
	}

	risk, xerr := h.OrgRiskService.SetOverride(c.Request.Context(), orgID, state, reason, *adminID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.logOrgRiskAction(c, *adminID, orgID, "set_org_risk_override", map[string]any{
		"state": string(state), "reason": reason,
	})
	c.JSON(http.StatusOK, risk)
}

// AdminClearOrgRiskOverride drops the pin and hands the posture back to the
// score, re-derived from the evidence still on file.
func (h *Handler) AdminClearOrgRiskOverride(c *gin.Context) {
	adminID := middleware.GetAdminUserID(c)
	if adminID == nil {
		errx.JSON(c, errx.ErrUnauthorized)
		return
	}
	orgID, ok := adminOrgParam(c)
	if !ok {
		return
	}
	if h.OrgRiskService == nil {
		errx.JSON(c, errx.New(errx.ServiceUnavailable, "risk posture is not available on this instance"))
		return
	}

	risk, xerr := h.OrgRiskService.ClearOverride(c.Request.Context(), orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.logOrgRiskAction(c, *adminID, orgID, "clear_org_risk_override", nil)
	c.JSON(http.StatusOK, risk)
}

// AdminClearOrgRiskSignal retracts one detector's finding. Reviewing a
// workspace and finding the evidence wrong has to be able to remove it, or the
// score never falls and the posture returns the moment a pin is lifted.
func (h *Handler) AdminClearOrgRiskSignal(c *gin.Context) {
	adminID := middleware.GetAdminUserID(c)
	if adminID == nil {
		errx.JSON(c, errx.ErrUnauthorized)
		return
	}
	orgID, ok := adminOrgParam(c)
	if !ok {
		return
	}
	if h.OrgRiskService == nil {
		errx.JSON(c, errx.New(errx.ServiceUnavailable, "risk posture is not available on this instance"))
		return
	}
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		errx.JSON(c, errx.New(errx.BadRequest, "which signal?"))
		return
	}

	risk, xerr := h.OrgRiskService.ClearSignal(c.Request.Context(), orgID, key)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	h.logOrgRiskAction(c, *adminID, orgID, "clear_org_risk_signal", map[string]any{"signal": key})
	c.JSON(http.StatusOK, risk)
}

// adminOrgParam resolves the :id path parameter, answering the request itself
// when it is not a uuid.
func adminOrgParam(c *gin.Context) (uuid.UUID, bool) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid organization ID"))
		return uuid.Nil, false
	}
	return orgID, true
}

// logOrgRiskAction records the operator action. The posture transition itself
// rides the audit spine from the service; this is the platform-admin trail,
// which is who-did-what across tenants rather than within one.
func (h *Handler) logOrgRiskAction(c *gin.Context, adminID, orgID uuid.UUID, action string, details map[string]any) {
	if h.AdminService == nil {
		return
	}
	h.AdminService.LogAdminAction(c.Request.Context(), adminID, action, "organization", &orgID,
		details, c.ClientIP(), c.Request.UserAgent())
}
