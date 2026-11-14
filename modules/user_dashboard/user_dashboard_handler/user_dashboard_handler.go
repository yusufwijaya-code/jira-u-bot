package user_dashboard_handler

import "github.com/gin-gonic/gin"

type UserDashboardHandler interface {
	// GetSettings returns the caller's notification settings, mute status
	// and Jira credential status as JSON (the Pengaturan page).
	GetSettings(c *gin.Context)
	// UpdateNotificationSettings saves tiers, overage behavior, toggles and
	// quiet hours (everything but mute).
	UpdateNotificationSettings(c *gin.Context)
	// Mute / Unmute start or clear a temporary mute instantly.
	Mute(c *gin.Context)
	Unmute(c *gin.Context)
	// GetBoard, GetTeam and GetOverdue return JSON scoped to the logged-in
	// user's own tickets (email taken from the session, admin included).
	GetBoard(c *gin.Context)
	GetTeam(c *gin.Context)
	GetOverdue(c *gin.Context)
	// TransitionTicket moves one of the caller's own tickets via the same
	// path as the bot's /u-geser (which enforces ownership).
	TransitionTicket(c *gin.Context)
	SaveCredential(c *gin.Context)
	DeleteCredential(c *gin.Context)
}
