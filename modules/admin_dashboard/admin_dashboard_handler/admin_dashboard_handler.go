package admin_dashboard_handler

import "github.com/gin-gonic/gin"

type AdminDashboardHandler interface {
	UserList(c *gin.Context)
	ToggleActive(c *gin.Context)
	EditJiraAccountID(c *gin.Context)
	DeleteUser(c *gin.Context)
	DeleteUserCredential(c *gin.Context)
	BroadcastMessage(c *gin.Context)
}
