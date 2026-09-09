package handler

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

// The join script is served by the instance itself, not from warmbly.com, so a
// self-hosted fleet never depends on a vendor host being reachable and always
// gets a script that matches the backend it is joining.
//
//go:embed nodescript/join.sh
var joinScript string

// ServeJoinScript is the front door for adding a machine to the fleet:
//
//	curl -fsSL https://<instance>/join.sh | sh -s -- --url ... --token ... --role worker
//
// Unauthenticated on purpose. The script itself is not a secret and does
// nothing without a valid join token; requiring credentials to read it would
// only mean pasting them twice.
func (h *Handler) ServeJoinScript(c *gin.Context) {
	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.String(http.StatusOK, joinScript)
}
