package auth

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/auth/auth_handler"
)

// Router accepts gin.IRouter (not *gin.Engine) so the caller can mount it
// under a sub-group (the app's BasePath) instead of domain root.
func Router(r gin.IRouter, handler auth_handler.AuthHandler) {
	google := r.Group("/auth/google")
	google.GET("/login", handler.Login)
	google.GET("/callback", handler.Callback)

	r.GET("/auth/logout", handler.Logout)
}
