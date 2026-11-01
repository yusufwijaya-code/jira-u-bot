package auth_middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"jira-u-bot/config"
	"jira-u-bot/modules/auth/auth_dto"
	"jira-u-bot/modules/auth/auth_service"
)

// CurrentUserKey is the gin.Context key AuthMiddleware stores the resolved
// auth_dto.Session under — handlers read it via c.MustGet(CurrentUserKey).
const CurrentUserKey = "currentUser"

const sessionCookieName = "session_token"

// AuthMiddleware implements spec AC1.6/AC1.7: every request under
// /dashboard/** must have a valid, non-expired session for a flagActive
// user — resolved fresh from the DB every request (not cached in the
// cookie), so a deactivated-mid-session user is blocked on their very next
// request.
func AuthMiddleware(authService auth_service.AuthService, cfg *config.AppConfig) gin.HandlerFunc {
	loginURL := cfg.BasePath + "/auth/google/login"
	return func(c *gin.Context) {
		token, err := c.Cookie(sessionCookieName)
		if err != nil || token == "" {
			c.Redirect(http.StatusFound, loginURL)
			c.Abort()
			return
		}

		session, found, err := authService.FindSession(c.Request.Context(), token)
		if err != nil || !found || !session.FlagActive {
			c.Redirect(http.StatusFound, loginURL)
			c.Abort()
			return
		}

		c.Set(CurrentUserKey, *session)
		c.Next()
	}
}

// AdminMiddleware implements spec AC1.8 — must run AFTER AuthMiddleware
// (reads CurrentUserKey it sets). Role is re-checked by email comparison
// every request, never cached (spec AC1.6).
func AdminMiddleware(cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := c.Get(CurrentUserKey)
		if !ok {
			// Misconfiguration (mounted without AuthMiddleware first) — fail
			// closed with 500 rather than panic through gin.Recovery.
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		if session.(auth_dto.Session).Email != cfg.AdminEmail {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}
