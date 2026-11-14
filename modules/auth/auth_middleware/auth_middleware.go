package auth_middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nudge/config"
	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_service"
)

// CurrentUserKey is the gin.Context key AuthMiddleware/AuthMiddlewareAPI
// stores the resolved auth_dto.Session under — handlers read it via
// c.MustGet(CurrentUserKey).
const CurrentUserKey = "currentUser"

const sessionCookieName = "session_token"

// AuthMiddlewareAPI implements spec AC1.6/AC1.7: every request must carry a
// valid, non-expired session for a flagActive user — resolved fresh from
// the DB every request (not cached in the cookie). Unauthenticated
// requests get a 401 JSON, which the Angular frontend's interceptor turns
// into a redirect to the login screen.
func AuthMiddlewareAPI(authService auth_service.AuthService, cfg *config.AppConfig) gin.HandlerFunc {
	return authMiddleware(authService, func(c *gin.Context) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	})
}

func authMiddleware(authService auth_service.AuthService, onUnauthorized gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(sessionCookieName)
		if err != nil || token == "" {
			onUnauthorized(c)
			c.Abort()
			return
		}

		session, found, err := authService.FindSession(c.Request.Context(), token)
		if err != nil || !found || !session.FlagActive {
			onUnauthorized(c)
			c.Abort()
			return
		}

		c.Set(CurrentUserKey, *session)
		c.Next()
	}
}

// AdminMiddleware implements spec AC1.8 — must run AFTER AuthMiddleware or
// AuthMiddlewareAPI (reads CurrentUserKey either one sets). Role is
// re-checked by email comparison every request, never cached.
func AdminMiddleware(cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := c.Get(CurrentUserKey)
		if !ok {
			// Misconfiguration (mounted without an auth middleware first) —
			// fail closed with 500 rather than panic through gin.Recovery.
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
