package middleware

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// InternalAuthMiddleware protects backend-to-backend endpoints (the worker DEK
// fetch is the first user) with a static bearer token sourced from the
// INTERNAL_API_TOKEN env var. Constant-time compare to defeat timing oracles.
//
// This is a deliberately simple primitive — workers and backend share one
// secret out-of-band (env var in both processes). Task #9 will replace this
// with per-worker JWTs minted at registration time.
//
// If INTERNAL_API_TOKEN is unset, every request is rejected — fail closed.
func (h *Handler) InternalAuthMiddleware() gin.HandlerFunc {
	return internalAuth
}

var (
	internalTokenOnce sync.Once
	internalToken     []byte
)

func loadInternalToken() {
	if v := os.Getenv("INTERNAL_API_TOKEN"); v != "" {
		internalToken = []byte(v)
	}
}

func internalAuth(c *gin.Context) {
	internalTokenOnce.Do(loadInternalToken)
	if len(internalToken) == 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "internal auth not configured"})
		return
	}
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
		return
	}
	provided := []byte(strings.TrimPrefix(header, "Bearer "))
	if subtle.ConstantTimeCompare(provided, internalToken) != 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid bearer token"})
		return
	}
	c.Next()
}

// NodeBrokerAuthMiddleware protects the two endpoints that perform a
// privileged operation on a caller's behalf: opening a sealed data key, and
// signing a blob operation.
//
// They are a step up from the rest of the internal API, which only moves
// records around. A caller here gets plaintext key material and a URL into the
// object store, so the token that opens them should not have to be the same
// one the tracking and forms services carry: those are internet-facing, and
// widening what their credential is worth is the whole risk.
//
// NODE_BROKER_TOKEN is that separate credential. It falls back to
// INTERNAL_API_TOKEN when unset, so an existing single-token deployment keeps
// working, and a split deployment can hand nodes something the edge services
// never see.
func (h *Handler) NodeBrokerAuthMiddleware() gin.HandlerFunc {
	return nodeBrokerAuth
}

var (
	brokerTokenOnce sync.Once
	brokerToken     []byte
)

func loadBrokerToken() {
	if v := os.Getenv("NODE_BROKER_TOKEN"); v != "" {
		brokerToken = []byte(v)
		return
	}
	brokerToken = []byte(os.Getenv("INTERNAL_API_TOKEN"))
}

func nodeBrokerAuth(c *gin.Context) {
	brokerTokenOnce.Do(loadBrokerToken)
	if len(brokerToken) == 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "internal auth not configured"})
		return
	}
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
		return
	}
	provided := []byte(strings.TrimPrefix(header, "Bearer "))
	if subtle.ConstantTimeCompare(provided, brokerToken) != 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid bearer token"})
		return
	}
	c.Next()
}
