package user_dashboard_handler

import "github.com/gin-gonic/gin"

type UserDashboardHandler interface {
	// GetBoard returns My Board as JSON — consumed by the Angular
	// frontend's board.service.ts (Tech Spec "Angular Dashboard Revamp",
	// Repository 1 Chapter 2).
	GetBoard(c *gin.Context)
	// GetTeam returns the Team view as JSON (Tech Spec "Angular Dashboard
	// Revamp", Repository 1 Chapter 3).
	GetTeam(c *gin.Context)
	TeamView(c *gin.Context)
	OverdueView(c *gin.Context)
	UpdatePreference(c *gin.Context)
	TransitionTicket(c *gin.Context)
	SaveCredential(c *gin.Context)
	DeleteCredential(c *gin.Context)
}
