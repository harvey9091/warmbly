package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

const PoolLinkInstanceKey = "pool_link_instance"

// PoolLinkAuthMiddleware authenticates a linked instance by bearer token; no user session exists here.
func (h *Handler) PoolLinkAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.PoolLinkService == nil {
			errx.JSON(c, errx.New(errx.NotImplemented, "pool link is not enabled on this instance"))
			c.Abort()
			return
		}
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") {
			errx.JSON(c, errx.ErrUnauthorized)
			c.Abort()
			return
		}
		inst, xerr := h.PoolLinkService.AuthenticateInstance(c.Request.Context(), strings.TrimPrefix(raw, "Bearer "), c.GetHeader("X-Warmbly-Instance-Version"))
		if xerr != nil {
			errx.JSON(c, xerr)
			c.Abort()
			return
		}
		c.Set(PoolLinkInstanceKey, inst)
		c.Next()
	}
}

// GetPoolLinkInstance returns the authenticated instance, or nil.
func GetPoolLinkInstance(c *gin.Context) *models.PoolLinkInstance {
	if v, ok := c.Get(PoolLinkInstanceKey); ok {
		if inst, ok := v.(*models.PoolLinkInstance); ok {
			return inst
		}
	}
	return nil
}
