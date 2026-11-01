package main

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/config"
	"jira-u-bot/modules/admin_dashboard"
	"jira-u-bot/modules/admin_dashboard/admin_dashboard_handler"
	"jira-u-bot/modules/auth"
	"jira-u-bot/modules/auth/auth_handler"
	"jira-u-bot/modules/auth/auth_middleware"
	"jira-u-bot/modules/auth/auth_service"
	"jira-u-bot/modules/user_dashboard"
	"jira-u-bot/modules/user_dashboard/user_dashboard_handler"
)

func SetupRouter(
	cfg *config.AppConfig,
	authSvc auth_service.AuthService,
	authHandler auth_handler.AuthHandler,
	userDashboardHandler user_dashboard_handler.UserDashboardHandler,
	adminDashboardHandler admin_dashboard_handler.AdminDashboardHandler,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	auth.Router(r, authHandler)

	dashboardPages := r.Group("/dashboard")
	dashboardPages.Use(auth_middleware.AuthMiddleware(authSvc))
	dashboardAPI := r.Group("/api/dashboard")
	dashboardAPI.Use(auth_middleware.AuthMiddleware(authSvc))
	user_dashboard.Router(dashboardPages, dashboardAPI, userDashboardHandler)

	dashboardAdmin := dashboardPages.Group("/admin")
	dashboardAdmin.Use(auth_middleware.AdminMiddleware(cfg))
	admin_dashboard.Router(dashboardAdmin, adminDashboardHandler)

	return r
}
