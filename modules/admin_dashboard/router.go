package admin_dashboard

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/admin_dashboard/admin_dashboard_handler"
)

// Router mounts the Admin Dashboard under group — the caller is
// responsible for applying AuthMiddleware AND AdminMiddleware to group
// before passing it here (spec AC6.6), same division of responsibility as
// auth.Router/user_dashboard.Router not knowing about sessions/roles.
func Router(group *gin.RouterGroup, handler admin_dashboard_handler.AdminDashboardHandler) {
	group.GET("", handler.UserList)
	group.POST("/users/:userID/active", handler.ToggleActive)
	group.POST("/users/:userID/jira-account-id", handler.EditJiraAccountID)
	group.DELETE("/users/:userID/credential", handler.DeleteUserCredential)
	group.DELETE("/users/:userID", handler.DeleteUser)
	group.POST("/broadcast", handler.BroadcastMessage)
}
