package user_dashboard_handler

import "github.com/gin-gonic/gin"

type UserDashboardHandler interface {
	Dashboard(c *gin.Context)
	TeamView(c *gin.Context)
	OverdueView(c *gin.Context)
	UpdatePreference(c *gin.Context)
	TransitionTicket(c *gin.Context)
	SaveCredential(c *gin.Context)
	DeleteCredential(c *gin.Context)
}
