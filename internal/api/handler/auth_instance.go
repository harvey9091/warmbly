package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/version"
)

// InstanceVersion answers GET /v1/auth/instance for any signed-in member of a
// self-hosted instance: which Warmbly this is and whether a newer one exists.
// It carries no updater detail and no log; applying the update lives in the
// admin panel. Hosted deployments answer self_hosted=false and nothing else,
// so the dashboard has nothing to show there.
type instanceVersionResponse struct {
	SelfHosted      bool                   `json:"self_hosted"`
	Version         string                 `json:"version,omitempty"`
	Commit          string                 `json:"commit,omitempty"`
	UpdateAvailable bool                   `json:"update_available"`
	Latest          *instanceLatestRelease `json:"latest,omitempty"`
	CheckedAt       *time.Time             `json:"checked_at,omitempty"`
}

type instanceLatestRelease struct {
	Tag         string    `json:"tag"`
	HTMLURL     string    `json:"html_url,omitempty"`
	PublishedAt time.Time `json:"published_at,omitempty"`
}

func (h *Handler) InstanceVersion(c *gin.Context) {
	if !config.SelfHosted() {
		c.JSON(http.StatusOK, instanceVersionResponse{SelfHosted: false})
		return
	}
	resp := instanceVersionResponse{
		SelfHosted: true,
		Version:    version.String(),
		Commit:     version.ShortCommit(),
	}
	if h.UpdatesService != nil {
		st := h.UpdatesService.State(c.Request.Context(), false)
		resp.UpdateAvailable = st.UpdateAvailable
		if st.Latest != nil {
			resp.Latest = &instanceLatestRelease{Tag: st.Latest.Tag, HTMLURL: st.Latest.HTMLURL, PublishedAt: st.Latest.PublishedAt}
		}
		if !st.CheckedAt.IsZero() {
			t := st.CheckedAt
			resp.CheckedAt = &t
		}
	}
	c.JSON(http.StatusOK, resp)
}
