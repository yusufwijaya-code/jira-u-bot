package auth

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/auth/auth_handler"
)

func Router(r *gin.Engine, handler auth_handler.AuthHandler) {
	google := r.Group("/auth/google")
	google.GET("/login", handler.Login)
	google.GET("/callback", handler.Callback)

	r.GET("/auth/logout", handler.Logout)
}
