package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/api/middleware"
	"github.com/warmbly/warmbly/internal/app/delegation"
	"github.com/warmbly/warmbly/internal/app/mailboximport"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// importCaller is the caller's workspace and user for the session-only mailbox source routes.
func importCaller(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.Handle(c, errx.ErrNoOrganization)
		return uuid.Nil, uuid.Nil, false
	}
	userID, err := middleware.GetUserUUID(c)
	if err != nil {
		errx.Handle(c, errx.ErrUser)
		return uuid.Nil, uuid.Nil, false
	}
	return *orgID, userID, true
}

func (h *Handler) grantsReady(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	if h.DelegationService == nil {
		errx.Handle(c, errx.ErrNotFound)
		return uuid.Nil, uuid.Nil, false
	}
	return importCaller(c)
}

// GetMailboxGrantConfig is GET /emails/grants/config: which admin connections
// this instance can take, and what a Google administrator has to authorize.
func (h *Handler) GetMailboxGrantConfig(c *gin.Context) {
	if _, _, ok := h.grantsReady(c); !ok {
		return
	}
	c.JSON(http.StatusOK, h.DelegationService.Config())
}

// ListMailboxGrants is GET /emails/grants.
func (h *Handler) ListMailboxGrants(c *gin.Context) {
	orgID, _, ok := h.grantsReady(c)
	if !ok {
		return
	}
	list, xerr := h.DelegationService.List(c.Request.Context(), orgID)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

type startGoogleGrantRequest struct {
	Domain     string `json:"domain"`
	AdminEmail string `json:"admin_email"`
}

// StartGoogleMailboxGrant is POST /emails/grants/google/start: how this
// workspace proves it controls the domain (an administrator's sign-in, or a
// DNS TXT record unique to the workspace).
func (h *Handler) StartGoogleMailboxGrant(c *gin.Context) {
	orgID, userID, ok := h.grantsReady(c)
	if !ok {
		return
	}
	var req startGoogleGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.Handle(c, errx.ErrInvalid)
		return
	}
	start, xerr := h.DelegationService.StartGoogle(c.Request.Context(), orgID, userID, req.Domain, req.AdminEmail)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}
	c.JSON(http.StatusOK, start)
}

// FinishGoogleMailboxGrant is POST /emails/grants/google/finish. A sign-in
// state is single-use; repeating a DNS finish re-verifies the same grant.
func (h *Handler) FinishGoogleMailboxGrant(c *gin.Context) {
	orgID, userID, ok := h.grantsReady(c)
	if !ok {
		return
	}
	var req delegation.GoogleFinish
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.Handle(c, errx.ErrInvalid)
		return
	}
	g, xerr := h.DelegationService.FinishGoogle(c.Request.Context(), orgID, userID, req)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionCreate, models.AuditEntityMailboxGrant, &g.ID, nil, map[string]string{"provider": g.Provider, "tenant": g.Tenant})
	c.JSON(http.StatusCreated, g)
}

// StartMicrosoftMailboxGrant is POST /emails/grants/microsoft/start: the admin consent URL.
func (h *Handler) StartMicrosoftMailboxGrant(c *gin.Context) {
	orgID, userID, ok := h.grantsReady(c)
	if !ok {
		return
	}
	u, state, xerr := h.DelegationService.StartMicrosoft(c.Request.Context(), orgID, userID)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": u, "state": state})
}

type finishMicrosoftGrantRequest struct {
	State string `json:"state"`
	Code  string `json:"code"`
}

// FinishMicrosoftMailboxGrant is POST /emails/grants/microsoft/finish. The state
// is single-use, so a repeat is refused rather than recorded twice.
func (h *Handler) FinishMicrosoftMailboxGrant(c *gin.Context) {
	orgID, userID, ok := h.grantsReady(c)
	if !ok {
		return
	}
	var req finishMicrosoftGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.Handle(c, errx.ErrInvalid)
		return
	}
	g, xerr := h.DelegationService.FinishMicrosoft(c.Request.Context(), orgID, userID, req.State, req.Code)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionCreate, models.AuditEntityMailboxGrant, &g.ID, nil, map[string]string{"provider": g.Provider, "tenant": g.Tenant})
	c.JSON(http.StatusCreated, g)
}

func (h *Handler) grantID(c *gin.Context) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	orgID, userID, ok := h.grantsReady(c)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.Handle(c, errx.ErrUuid)
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return orgID, userID, id, true
}

// GetMailboxGrant is GET /emails/grants/:id.
func (h *Handler) GetMailboxGrant(c *gin.Context) {
	orgID, _, id, ok := h.grantID(c)
	if !ok {
		return
	}
	g, xerr := h.DelegationService.Get(c.Request.Context(), orgID, id)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}
	c.JSON(http.StatusOK, g)
}

// CheckMailboxGrant is POST /emails/grants/:id/check: verify again, and put
// the grant's mailboxes back to work if it recovered.
func (h *Handler) CheckMailboxGrant(c *gin.Context) {
	orgID, _, id, ok := h.grantID(c)
	if !ok {
		return
	}
	g, xerr := h.DelegationService.Recheck(c.Request.Context(), orgID, id)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}
	c.JSON(http.StatusOK, g)
}

// DeleteMailboxGrant is DELETE /emails/grants/:id. Its mailboxes stop.
func (h *Handler) DeleteMailboxGrant(c *gin.Context) {
	orgID, userID, id, ok := h.grantID(c)
	if !ok {
		return
	}
	if xerr := h.DelegationService.Delete(c.Request.Context(), orgID, userID, id); xerr != nil {
		errx.Handle(c, xerr)
		return
	}
	h.auditOrg(c, models.AuditActionDelete, models.AuditEntityMailboxGrant, &id, nil, nil)
	c.Status(http.StatusNoContent)
}

// ListMailboxGrantUsers is GET /emails/grants/:id/users: the granted directory.
func (h *Handler) ListMailboxGrantUsers(c *gin.Context) {
	orgID, _, id, ok := h.grantID(c)
	if !ok {
		return
	}
	users, xerr := h.DelegationService.Users(c.Request.Context(), orgID, id)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": users})
}

type connectGrantRequest struct {
	UserIDs []string                    `json:"user_ids"`
	All     bool                        `json:"all"`
	Options models.MailboxImportOptions `json:"options"`
}

// ConnectMailboxGrantUsers is POST /emails/grants/:id/connect: import the picked
// users (or every enabled one not yet here) as a background import.
func (h *Handler) ConnectMailboxGrantUsers(c *gin.Context) {
	orgID, userID, id, ok := h.grantID(c)
	if !ok {
		return
	}
	var req connectGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.Handle(c, errx.ErrInvalid)
		return
	}
	g, xerr := h.DelegationService.Get(c.Request.Context(), orgID, id)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}
	users, xerr := h.DelegationService.Users(c.Request.Context(), orgID, id)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}
	want := map[string]bool{}
	for _, u := range req.UserIDs {
		want[u] = true
	}
	rows := make([]mailboximport.ListRow, 0)
	for _, u := range users {
		pick := want[u.ID] || (req.All && u.Enabled && (!u.Connected || u.Upgrade))
		if !pick {
			continue
		}
		gid := g.ID
		rows = append(rows, mailboximport.ListRow{Email: u.Email, Name: u.Name, GrantID: &gid})
	}
	label := g.Tenant
	if g.Provider == models.GrantProviderMicrosoft && len(g.Domains) > 0 {
		label = strings.Join(g.Domains, ", ")
	}
	imp, xerr := h.MailboxImportService.CreateFromList(c.Request.Context(), mailboximport.ListInput{
		OrgID: orgID, UserID: userID, Source: "workspace", Vendor: g.Provider, Label: label, Rows: rows, Options: req.Options,
	})
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}
	c.JSON(http.StatusCreated, imp)
}

// GetSigninMigration is GET /emails/grants/migration: the workspace's mailboxes
// still on per-mailbox Google sign-in, by domain, with where each moves.
func (h *Handler) GetSigninMigration(c *gin.Context) {
	orgID, _, ok := importCaller(c)
	if !ok {
		return
	}
	if h.DelegationService == nil {
		c.JSON(http.StatusOK, gin.H{"data": []models.SigninMigration{}, "total": 0})
		return
	}
	groups, xerr := h.DelegationService.SigninMigration(c.Request.Context(), orgID)
	if xerr != nil {
		errx.Handle(c, xerr)
		return
	}
	total := 0
	for _, g := range groups {
		total += len(g.Mailboxes)
	}
	c.JSON(http.StatusOK, gin.H{"data": groups, "total": total})
}
