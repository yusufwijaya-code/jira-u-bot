package admin_dashboard_handler

import "github.com/gin-gonic/gin"

type AdminDashboardHandler interface {
	UserList(c *gin.Context)
	// ListUsers returns the admin user list as JSON (Tech Spec "Angular
	// Dashboard Revamp", Repository 1 Chapter 5).
	ListUsers(c *gin.Context)
	ToggleActive(c *gin.Context)
	EditJiraAccountID(c *gin.Context)
	DeleteUser(c *gin.Context)
	DeleteUserCredential(c *gin.Context)
	BroadcastMessage(c *gin.Context)
}
