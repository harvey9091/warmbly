package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/api/middleware"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// emailSyncResponse is GET /emails/:id/sync: where the mailbox's import
// stands, whether fair use is holding it, the budget it runs under, the
// folders the owner excluded, and the folders the sync has seen so a client
// can offer them by name.
type emailSyncResponse struct {
	// State is null until the worker has reported once.
	State  *models.SyncState `json:"state"`
	Policy models.SyncPolicy `json:"policy"`
	// SkipFolders is the stored skip list; the same value the policy carries,
	// surfaced where PUT writes it.
	SkipFolders []string `json:"skip_folders"`
	// Folders is what the worker last listed on the server, INBOX first.
	// Empty for Gmail and Outlook, which have no IMAP folder list.
	Folders []models.SyncFolder `json:"folders"`
}

// GetEmailSync reports a mailbox's sync progress, fair-use status and folder
// settings.
func (h *Handler) GetEmailSync(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.ErrUnauthorized)
		return
	}
	state, policy, xerr := h.EmailService.GetSyncState(c.Request.Context(), orgID.String(), c.Param("id"))
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	folders, xerr := h.EmailService.GetSyncFolders(c.Request.Context(), orgID.String(), c.Param("id"))
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	skip := policy.SkipFolders
	if skip == nil {
		skip = []string{}
	}
	c.JSON(http.StatusOK, emailSyncResponse{State: state, Policy: policy, SkipFolders: skip, Folders: folders})
}

// UpdateEmailSync replaces the folders the mailbox's sync leaves alone.
//
// Naturally idempotent: the body is the desired list, so a retry converges
// on the same stored value and no Idempotency-Key is needed.
// PUT /emails/:id/sync
func (h *Handler) UpdateEmailSync(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.ErrUnauthorized)
		return
	}
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errx.Handle(c, errx.ErrUuid)
		return
	}

	var body models.UpdateSyncSettings
	if err := c.ShouldBindJSON(&body); err != nil {
		errx.Handle(c, errx.ErrInvalid)
		return
	}

	skip, xerr := h.EmailService.UpdateSyncSettings(c.Request.Context(), orgID.String(), accountID.String(), &body)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}

	h.auditOrg(c, models.AuditActionUpdate, models.AuditEntityEmailAccount, &accountID, nil, map[string]string{"sync_skip_folders": "updated"})
	c.JSON(http.StatusOK, gin.H{"skip_folders": skip})
}
