package main

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nudge/config"
	"nudge/modules/admin_dashboard"
	"nudge/modules/admin_dashboard/admin_dashboard_handler"
	"nudge/modules/auth"
	"nudge/modules/auth/auth_handler"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/auth/auth_service"
	"nudge/modules/sprint_report"
	"nudge/modules/sprint_report/sprint_report_handler"
	"nudge/modules/user_dashboard"
	"nudge/modules/user_dashboard/user_dashboard_handler"
)

// SetupRouter mounts every route under cfg.BasePath — labcup.tech hosts
// several repos behind one nginx vhost, each claiming its own prefix with
// NO stripping by nginx (matching the pre-existing /nudge convention
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
	// Bare BasePath (someone just types/bookmarks the /nudge prefix
	// with nothing after it) goes straight to the dashboard — AuthMiddleware
	// further down the chain bounces unauthenticated visitors to login same
	// as hitting /dashboard directly.
	base.GET("", redirectToDashboardHandler(cfg))
	r.NoRoute(notFoundHandler())

	auth.Router(base, authHandler)

	apiAuth := base.Group("/api/auth")
	apiAuth.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	apiAuth.GET("/me", authHandler.Me)

	dashboardAPI := base.Group("/api/dashboard")
	dashboardAPI.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	user_dashboard.Router(dashboardAPI, userDashboardHandler)
	sprint_report.Router(dashboardAPI, sprintReportHandler)

	dashboardAdmin := base.Group("/dashboard/admin")
	dashboardAdmin.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	dashboardAdmin.Use(auth_middleware.AdminMiddleware(cfg))

	apiDashboardAdmin := dashboardAPI.Group("/admin")
	apiDashboardAdmin.Use(auth_middleware.AdminMiddleware(cfg))

	admin_dashboard.Router(dashboardAdmin, apiDashboardAdmin, adminDashboardHandler)

	return r
}

func redirectToDashboardHandler(cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Redirect(http.StatusFound, cfg.FrontendURL+cfg.BasePath+"/dashboard")
	}
}

// notFoundHandler is registered via r.NoRoute (not a BasePath-scoped group —
// gin has no NoRoute on *RouterGroup), but that's fine: nginx only ever
// forwards this app requests already under cfg.BasePath, so any unmatched
// path reaching this process is a genuine 404 for it. The frontend is a
// separate Angular app, so the backend only ever answers with JSON.
func notFoundHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	}
}
