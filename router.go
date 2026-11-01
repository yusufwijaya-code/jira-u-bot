package main

import (
	"html/template"
	"net/http"

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
	// Bare BasePath (someone just types/bookmarks the /jira-notif prefix
	// with nothing after it) goes straight to the dashboard — AuthMiddleware
	// further down the chain bounces unauthenticated visitors to login same
	// as hitting /dashboard directly.
	base.GET("", redirectToDashboardHandler(cfg))
	r.NoRoute(notFoundHandler(cfg))

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

func redirectToDashboardHandler(cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Redirect(http.StatusFound, cfg.BasePath+"/dashboard")
	}
}

var notFoundTemplate = template.Must(template.New("404").Parse(notFoundHTML))

// notFoundHandler is registered via r.NoRoute (not a BasePath-scoped group —
// gin has no NoRoute on *RouterGroup), but that's fine: nginx only ever
// forwards this app requests already under cfg.BasePath, so any unmatched
// path reaching this process is a genuine 404 for it.
func notFoundHandler(cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Status(http.StatusNotFound)
		notFoundTemplate.Execute(c.Writer, struct{ BasePath string }{cfg.BasePath})
	}
}

const notFoundHTML = `<!DOCTYPE html>
<html lang="id">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>404 - Jira U Bot</title>
<link href="https://fonts.googleapis.com/css2?family=REM:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
    :root { --blue-dark:#0e65a9; --blue-light:#eaf5ff; --text:#333; --text-muted:#6c6c6c; --border:#e6e6e6; }
    * { box-sizing: border-box; }
    body { margin:0; min-height:100vh; display:flex; align-items:center; justify-content:center; padding:24px; font-family:'REM',ui-sans-serif,system-ui,sans-serif; color:var(--text); background: radial-gradient(circle at top left, var(--blue-light), #ffffff 55%); }
    .card { width:100%; max-width:440px; background:#fff; border-radius:24px; padding:40px 36px; box-shadow:0 20px 45px -15px rgba(9,67,113,0.18); border:1px solid var(--border); text-align:center; }
    .code { font-size:3rem; font-weight:700; color:var(--blue-dark); margin:0; }
    h1 { font-size:1.2rem; font-weight:700; margin:8px 0 8px; color:var(--blue-dark); }
    p { color:var(--text-muted); font-size:0.9rem; margin:0 0 28px; line-height:1.5; }
    .btn { display:inline-flex; align-items:center; justify-content:center; width:100%; padding:13px 20px; font-family:inherit; font-size:1rem; font-weight:600; color:#fff; text-decoration:none; border-radius:999px; background:linear-gradient(to right, #148ae1, #00cfab); }
</style>
</head>
<body>
    <div class="card">
        <p class="code">404</p>
        <h1>Halaman tidak ditemukan</h1>
        <p>URL yang kamu buka tidak ada di Jira U Bot.</p>
        <a class="btn" href="{{.BasePath}}/dashboard">Kembali ke Dashboard</a>
    </div>
</body>
</html>`
