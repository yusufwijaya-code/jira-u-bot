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
	"jira-u-bot/modules/sprint_report"
	"jira-u-bot/modules/sprint_report/sprint_report_handler"
	"jira-u-bot/modules/user_dashboard"
	"jira-u-bot/modules/user_dashboard/user_dashboard_handler"
)

// SetupRouter mounts every route under cfg.BasePath — labcup.tech hosts
// several repos behind one nginx vhost, each claiming its own prefix with
// NO stripping by nginx (matching the pre-existing /jira-notif convention
// from the old registration page), so this app must own its full path
// itself rather than assume it's ever served from domain root.
func SetupRouter(
	cfg *config.AppConfig,
	authSvc auth_service.AuthService,
	authHandler auth_handler.AuthHandler,
	userDashboardHandler user_dashboard_handler.UserDashboardHandler,
	adminDashboardHandler admin_dashboard_handler.AdminDashboardHandler,
	sprintReportHandler sprint_report_handler.SprintReportHandler,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	base := r.Group(cfg.BasePath)

	base.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	auth.Router(base, authHandler)

	dashboardPages := base.Group("/dashboard")
	dashboardPages.Use(auth_middleware.AuthMiddleware(authSvc, cfg))
	dashboardAPI := base.Group("/api/dashboard")
	dashboardAPI.Use(auth_middleware.AuthMiddleware(authSvc, cfg))
	user_dashboard.Router(dashboardPages, dashboardAPI, userDashboardHandler)
	sprint_report.Router(dashboardPages, sprintReportHandler)

	dashboardAdmin := dashboardPages.Group("/admin")
	dashboardAdmin.Use(auth_middleware.AdminMiddleware(cfg))
	admin_dashboard.Router(dashboardAdmin, adminDashboardHandler)

	return r
}
