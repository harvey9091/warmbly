package middleware

import (
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/warmbly/warmbly/internal/errx"
)

const (
	googleIssuer = "https://accounts.google.com"
)

type OidcHandler struct {
	ServiceAccount string
	KeySet         keyfunc.Keyfunc
	AppEnv         string
	// Audience is this instance's public URL, which Cloud Tasks puts in the
	// token it mints for the webhook. Without it any token that service
	// account holds for any audience is accepted here. Empty leaves the check
	// off, for a deployment that cannot name its own URL.
	Audience string
}

func (h *OidcHandler) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.AppEnv == "dev" {
			c.Next()
			return
		}

		// No key set means the GCP Cloud Tasks OIDC path isn't configured (the
		// default local dispatcher calls handlers in-process). Fail closed
		// rather than deref a nil key set if a webhook request slips through.
		if h.KeySet == nil {
			errx.Handle(c, errx.ErrForbidden)
			return
		}

		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			errx.Handle(c, errx.ErrForbidden)
			return
		}

		tokenStr := strings.TrimPrefix(auth, "Bearer ")

		opts := []jwt.ParserOption{
			jwt.WithLeeway(10 * time.Second),
			jwt.WithValidMethods([]string{"RS256"}),
			jwt.WithExpirationRequired(),
		}
		if h.Audience != "" {
			opts = append(opts, jwt.WithAudience(h.Audience))
		}

		token, err := jwt.Parse(tokenStr, h.KeySet.Keyfunc, opts...)
		if err != nil {
			errx.Handle(c, errx.ErrForbidden)
			return
		}

		if !token.Valid {
			errx.Handle(c, errx.ErrForbidden)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			errx.Handle(c, errx.ErrForbidden)
			return
		}

		if iss, _ := claims.GetIssuer(); iss != googleIssuer {
			errx.Handle(c, errx.ErrForbidden)
			return
		}

		if sAccount, _ := claims.GetSubject(); sAccount != h.ServiceAccount {
			errx.Handle(c, errx.ErrForbidden)
			return
		}

		c.Next()
	}
}
