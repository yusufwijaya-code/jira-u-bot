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
	"nudge/modules/chat_app_activation"
	"nudge/modules/chat_log/chat_log_handler"
	"nudge/modules/sprint_report"
	"nudge/modules/sprint_report/sprint_report_handler"
	"nudge/modules/user_avatar/user_avatar_handler"
	"nudge/modules/user_dashboard"
	"nudge/modules/user_dashboard/user_dashboard_handler"
	"nudge/modules/user_leave"
	"nudge/modules/user_leave/user_leave_handler"
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
	adminUserSettings *user_dashboard_handler.SettingsHandler,
	userAvatars *user_avatar_handler.UserAvatarHandler,
	userLeaves *user_leave_handler.UserLeaveHandler,
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
	// Tech Spec "Papan Tiket & Cuti": the session user's leave days.
	user_leave.Router(dashboardAPI, userLeaves)
	// Jira profile pictures (Tech Spec "Admin Panel Revamp").
	dashboardAPI.GET("/me/avatar", userAvatars.Mine)

	dashboardAdmin := base.Group("/dashboard/admin")
	dashboardAdmin.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	dashboardAdmin.Use(auth_middleware.AdminMiddleware(cfg))

	apiDashboardAdmin := dashboardAPI.Group("/admin")
	apiDashboardAdmin.Use(auth_middleware.AdminMiddleware(cfg))

	admin_dashboard.Router(dashboardAdmin, apiDashboardAdmin, adminDashboardHandler, adminUserSettings)
	apiDashboardAdmin.GET("/users/:userID/avatar", userAvatars.ForUser)

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

// ChatAppRoutes are the Tech Spec "Nudge sebagai Google Chat App"
// endpoints. Events is nil when the Chat App is not configured.
type ChatAppRoutes struct {
	Events      gin.HandlerFunc
	Activation  *chat_app_activation.Handler
	AdminTarget user_dashboard_handler.TargetResolver
	AdminChange user_dashboard_handler.ChangeListener
}

// MountChatLog mounts the admin "Riwayat pesan" list (admin only).
func MountChatLog(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *chat_log_handler.Handler) {
	admin := r.Group(cfg.BasePath + "/api/dashboard/admin")
	admin.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg), auth_middleware.AdminMiddleware(cfg))
	admin.GET("/chat-logs", h.List)
	admin.GET("/chat-logs/conversations", h.Conversations)
	admin.POST("/chat-logs/send", h.Send)
}

// MountChatApp mounts the Chat App routes next to SetupRouter's.
func MountChatApp(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, routes ChatAppRoutes) {
	base := r.Group(cfg.BasePath)
	if routes.Events != nil {
		base.POST("/api/chat/events", routes.Events)
	}
	a := routes.Activation
	if a == nil {
		return
	}
	// Session routes. /auth/chat-app/* answers 401 JSON without a session;
	// it is only ever opened from the dashboard by a logged-in user.
	session := base.Group("")
	session.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	session.GET("/auth/chat-app/activate", a.Activate)
	session.GET("/auth/chat-app/callback", a.Callback)
	session.GET("/api/dashboard/chat-app/status", a.Status(user_dashboard_handler.SessionTarget))
	session.PUT("/api/dashboard/settings/chat-sender", a.SetSender(user_dashboard_handler.SessionTarget, nil))

	admin := base.Group("/api/dashboard/admin")
	admin.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg), auth_middleware.AdminMiddleware(cfg))
	admin.GET("/users/:userID/chat-app/status", a.Status(routes.AdminTarget))
	admin.PUT("/users/:userID/settings/chat-sender", a.SetSender(routes.AdminTarget, routes.AdminChange))
}
