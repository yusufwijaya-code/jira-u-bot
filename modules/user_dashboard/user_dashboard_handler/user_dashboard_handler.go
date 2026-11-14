package user_dashboard_handler

import "github.com/gin-gonic/gin"

type UserDashboardHandler interface {
	// GetSettings returns the caller's reminder tier preference and Jira
	// credential status as JSON — consumed by the Angular frontend's
	// settings page.
	GetSettings(c *gin.Context)
	// GetBoard, GetTeam and GetOverdue return JSON scoped to the logged-in
	// user's own tickets (email taken from the session, admin included).
	GetBoard(c *gin.Context)
	GetTeam(c *gin.Context)
	GetOverdue(c *gin.Context)
	// TransitionTicket moves one of the caller's own tickets via the same
	// path as the bot's /u-geser (which enforces ownership).
	TransitionTicket(c *gin.Context)
	UpdatePreference(c *gin.Context)
	SaveCredential(c *gin.Context)
	DeleteCredential(c *gin.Context)
}
