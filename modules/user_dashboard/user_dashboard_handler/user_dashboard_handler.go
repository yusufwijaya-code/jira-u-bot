package user_dashboard_handler

import "github.com/gin-gonic/gin"

type UserDashboardHandler interface {
	// GetSettings returns the caller's notification settings, mute status
	// and Jira credential status as JSON (the Pengaturan page).
	GetSettings(c *gin.Context)
	// UpdateNotificationSettings saves tiers, overage behavior, toggles and
	// quiet hours (everything but mute).
	UpdateNotificationSettings(c *gin.Context)
	// UpdateNotificationsSwitch saves the master on/off switch (Tech Spec
	// "Notification On-Off Consent").
	UpdateNotificationsSwitch(c *gin.Context)
	// ResetNotificationSettings makes a board follow the default again.
	ResetNotificationSettings(c *gin.Context)
	// Mute / Unmute start or clear a mute instantly (global or one board).
	Mute(c *gin.Context)
	Unmute(c *gin.Context)
	// ListBoards returns the board picker entries; BoardAvatar proxies a
	// board's project avatar.
	ListBoards(c *gin.Context)
	BoardAvatar(c *gin.Context)
	// GetBoard, GetTeam and GetOverdue return JSON scoped to the logged-in
	// user's own tickets (email taken from the session, admin included).
	GetBoard(c *gin.Context)
	GetTeam(c *gin.Context)
	GetOverdue(c *gin.Context)
	// GetTransitions lists the moves the ticket's Jira workflow allows now
	// (Tech Spec "Papan Tiket & Cuti"); own active-sprint tickets only.
	GetTransitions(c *gin.Context)
	// TransitionTicket moves one of the caller's own tickets: by workflow
	// transition id (Papan Tiket), or by the legacy target slug through the
	// same path as the bot's /n-geser.
	TransitionTicket(c *gin.Context)
	SaveCredential(c *gin.Context)
	DeleteCredential(c *gin.Context)
}
