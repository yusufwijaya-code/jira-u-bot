package auth_middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"jira-u-bot/config"
	"jira-u-bot/modules/auth/auth_dto"
	"jira-u-bot/modules/auth/auth_service"
)

// CurrentUserKey is the gin.Context key AuthMiddleware/AuthMiddlewareAPI
// stores the resolved auth_dto.Session under — handlers read it via
// c.MustGet(CurrentUserKey).
const CurrentUserKey = "currentUser"

const sessionCookieName = "session_token"

// AuthMiddleware implements spec AC1.6/AC1.7: every request under the old
// server-rendered page routes must have a valid, non-expired session for a
// flagActive user — resolved fresh from the DB every request (not cached in
// the cookie). Unauthenticated requests are redirected to the app's own
// landing page. This variant stays in place only for as long as the old
// HTML page routes exist — once they're deleted (Angular Dashboard Revamp
// Plan C), only AuthMiddlewareAPI remains.
func AuthMiddleware(authService auth_service.AuthService, cfg *config.AppConfig) gin.HandlerFunc {
	loginURL := cfg.BasePath + "/auth/login"
	return authMiddleware(authService, func(c *gin.Context) {
		c.Redirect(http.StatusFound, loginURL)
	})
}

// AuthMiddlewareAPI is the JSON-API counterpart to AuthMiddleware — used
// for routes consumed by fetch()/HttpClient (Angular), where a 302 redirect
// to an HTML page is useless (the caller can't meaningfully follow it and
// the response body wouldn't be JSON). Same session validation, same
// CurrentUserKey contract, only the failure response differs: 401 JSON
// instead of a redirect.
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
