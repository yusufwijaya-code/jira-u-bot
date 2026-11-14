package admin_dashboard

import (
	"github.com/gin-gonic/gin"

	"nudge/modules/admin_dashboard/admin_dashboard_handler"
)

// Router mounts the Admin Dashboard's JSON actions on
// actionGroup (unchanged paths under the old /dashboard/admin/* prefix)
// and the new JSON user list on apiGroup (new GET /api/dashboard/admin/users
// path, Tech Spec "Angular Dashboard Revamp", Repository 1 Chapter 5) — the
// caller is responsible for applying AuthMiddleware/AuthMiddlewareAPI AND
// AdminMiddleware to both groups before passing them here, same division
// of responsibility as before.
func Router(actionGroup *gin.RouterGroup, apiGroup *gin.RouterGroup, handler admin_dashboard_handler.AdminDashboardHandler) {
	actionGroup.POST("/users/:userID/active", handler.ToggleActive)
	actionGroup.POST("/users/:userID/jira-account-id", handler.EditJiraAccountID)
	actionGroup.DELETE("/users/:userID/credential", handler.DeleteUserCredential)
	actionGroup.DELETE("/users/:userID", handler.DeleteUser)
	actionGroup.POST("/broadcast", handler.BroadcastMessage)

	apiGroup.GET("/users", handler.ListUsers)
}
