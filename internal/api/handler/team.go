package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/api/middleware"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// =====================
// Teams
// =====================

// addTeamMemberRequest is the body for POST /teams/:id/members. The user must
// already belong to the organization; that is validated in the service/repo.
type addTeamMemberRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
}

func (h *Handler) ListTeams(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.Handle(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}

	teams, xerr := h.TeamService.ListTeams(c.Request.Context(), *orgID)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": teams})
}

func (h *Handler) CreateTeam(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.Handle(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}

	var data models.CreateTeam
	if err := c.ShouldBindJSON(&data); err != nil {
		errx.Handle(c, errx.InvalidBody(err))
		return
	}

	team, xerr := h.TeamService.CreateTeam(c.Request.Context(), *orgID, &data)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}

	h.auditOrg(c, models.AuditActionCreate, models.AuditEntityTeam, &team.ID, nil, map[string]string{"name": team.Name})

	c.JSON(http.StatusCreated, team)
}

func (h *Handler) GetTeam(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.Handle(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	teamID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.Handle(c, errx.ErrUuid)
		return
	}

	team, xerr := h.TeamService.GetTeam(c.Request.Context(), *orgID, teamID)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}

	c.JSON(http.StatusOK, team)
}

func (h *Handler) UpdateTeam(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.Handle(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	teamID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.Handle(c, errx.ErrUuid)
		return
	}

	var data models.UpdateTeam
	if err := c.ShouldBindJSON(&data); err != nil {
		errx.Handle(c, errx.InvalidBody(err))
		return
	}

	team, xerr := h.TeamService.UpdateTeam(c.Request.Context(), *orgID, teamID, &data)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}

	h.auditOrg(c, models.AuditActionUpdate, models.AuditEntityTeam, &teamID, nil, nil)

	c.JSON(http.StatusOK, team)
}

func (h *Handler) DeleteTeam(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.Handle(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	teamID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.Handle(c, errx.ErrUuid)
		return
	}

	xerr := h.TeamService.DeleteTeam(c.Request.Context(), *orgID, teamID)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}

	h.auditOrg(c, models.AuditActionDelete, models.AuditEntityTeam, &teamID, nil, nil)

	c.Status(http.StatusNoContent)
}

// =====================
// Team Members
// =====================

func (h *Handler) AddTeamMember(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.Handle(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	teamID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.Handle(c, errx.ErrUuid)
		return
	}

	var data addTeamMemberRequest
	if err := c.ShouldBindJSON(&data); err != nil {
		errx.Handle(c, errx.InvalidBody(err))
		return
	}

	if xerr := h.TeamService.AddMember(c.Request.Context(), *orgID, teamID, data.UserID); xerr != nil {
		errx.Handle(c, xerr)
		return
	}

	team, xerr := h.TeamService.GetTeam(c.Request.Context(), *orgID, teamID)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}

	h.auditOrg(c, models.AuditActionAssign, models.AuditEntityTeam, &teamID, nil, map[string]string{"member_user_id": data.UserID.String()})

	c.JSON(http.StatusOK, team)
}

func (h *Handler) RemoveTeamMember(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.Handle(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}
	teamID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.Handle(c, errx.ErrUuid)
		return
	}
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		errx.Handle(c, errx.ErrUuid)
		return
	}

	if xerr := h.TeamService.RemoveMember(c.Request.Context(), *orgID, teamID, userID); xerr != nil {
		errx.Handle(c, xerr)
		return
	}

	h.auditOrg(c, models.AuditActionRemove, models.AuditEntityTeam, &teamID, nil, map[string]string{"member_user_id": userID.String()})

	c.Status(http.StatusNoContent)
}
