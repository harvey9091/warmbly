package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/warmbly/warmbly/internal/app/token"
	"github.com/warmbly/warmbly/internal/errx"
)

// RequireFreshAuth refuses a request whose session has not re-proved the
// account holder recently.
//
// CASA 2.4.1 asks for a full session plus re-authentication or a secondary
// check before a sensitive account change. Changing a password and disabling
// 2FA already demand a current credential. The changes gated here did not, and
// each of them either hands an attacker a durable credential of their own (an
// API key, a passkey registered to their device) or is irreversible from the
// victim's side (ownership transfer, scheduled deletion). A stolen access token
// was enough for all of them.
//
// The caller re-authenticates at POST /v1/auth/reauth and retries.
//
// This applies to session callers only. An API key and an OAuth token have no
// session and no way to present a second factor, and they are not the threat
// this addresses: the risk is a browser token lifted from a machine somebody
// walked away from. A key is already a deliberate, scoped, revocable grant that
// was itself minted from a confirmed session, its use is audited, and the
// permission gate on the route decides what it may do. Refusing them here would
// break every automation, the CLI included, to exceed what the control asks
// for.
func RequireFreshAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if authType, _ := c.Get(AuthTypeKey); authType == AuthTypeAPIKey || authType == AuthTypeOAuth {
			c.Next()
			return
		}

		session := GetSession(c)
		if session == nil {
			errx.JSON(c, errx.NewWithIdentifier(errx.Unauthorized, "reauth_required",
				"This action needs a signed-in session. Sign in and try again."))
			c.Abort()
			return
		}
		if session.ReauthAt == nil || time.Since(*session.ReauthAt) > token.ReauthWindow {
			errx.JSON(c, errx.NewWithIdentifier(errx.Forbidden, "reauth_required",
				"Confirm it is you before making this change."))
			c.Abort()
			return
		}
		c.Next()
	}
}
