package admin_dashboard

import (
	"github.com/gin-gonic/gin"

	"nudge/modules/admin_dashboard/admin_dashboard_handler"
	"nudge/modules/user_dashboard/user_dashboard_handler"
)

// Router mounts the Admin Dashboard's JSON actions on
// actionGroup (unchanged paths under the old /dashboard/admin/* prefix)
// and the JSON API on apiGroup (/api/dashboard/admin/*, Tech Spec "Angular
// Dashboard Revamp" + "Admin Panel Revamp") — the caller is responsible for
// applying AuthMiddleware/AuthMiddlewareAPI AND AdminMiddleware to both
// groups before passing them here, same division of responsibility as
// before.
//
// userSettings is the shared SettingsHandler resolving its target from
// :userID — it serves the same settings / mute / credential / board-list
// shapes as the user's own /api/dashboard endpoints.
func Router(actionGroup *gin.RouterGroup, apiGroup *gin.RouterGroup, handler admin_dashboard_handler.AdminDashboardHandler, userSettings *user_dashboard_handler.SettingsHandler) {
	actionGroup.POST("/users/:userID/active", handler.ToggleActive)
	actionGroup.POST("/users/:userID/jira-account-id", handler.EditJiraAccountID)
	actionGroup.DELETE("/users/:userID/credential", handler.DeleteUserCredential)
	actionGroup.DELETE("/users/:userID", handler.DeleteUser)
	actionGroup.POST("/broadcast", handler.BroadcastMessage)

	apiGroup.GET("/users", handler.ListUsers)
	apiGroup.GET("/users/:userID", handler.GetUser)
	apiGroup.GET("/users/:userID/boards", userSettings.ListBoards)
	apiGroup.GET("/users/:userID/settings", userSettings.GetSettings)
	apiGroup.PUT("/users/:userID/settings/notification", userSettings.UpdateNotification)
	apiGroup.DELETE("/users/:userID/settings/notification", userSettings.ResetNotification)
	apiGroup.POST("/users/:userID/settings/mute", userSettings.Mute)
	apiGroup.DELETE("/users/:userID/settings/mute", userSettings.Unmute)
	apiGroup.PUT("/users/:userID/credential", userSettings.SaveCredential)
}
