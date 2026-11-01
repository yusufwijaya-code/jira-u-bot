package main

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/config"
	"jira-u-bot/modules/auth"
	"jira-u-bot/modules/auth/auth_handler"
	"jira-u-bot/modules/auth/auth_middleware"
	"jira-u-bot/modules/auth/auth_service"
)

func SetupRouter(cfg *config.AppConfig, authSvc auth_service.AuthService, authHandler auth_handler.AuthHandler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	auth.Router(r, authHandler)

	// Temporary placeholder — Chapter 9 (User Dashboard) replaces this with
	// the real My Board/preference/credential UI. Its only job right now is
	// to prove AuthMiddleware actually gates access end to end.
	dashboard := r.Group("/dashboard")
	dashboard.Use(auth_middleware.AuthMiddleware(authSvc))
	dashboard.GET("", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "message": "logged in — User Dashboard lands in Chapter 9"})
	})
	dashboard.GET("/admin", auth_middleware.AdminMiddleware(cfg), func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "message": "logged in as admin — Admin Dashboard lands in Chapter 10"})
	})

	return r
}
