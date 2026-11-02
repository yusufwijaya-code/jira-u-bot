package user_dashboard

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/user_dashboard/user_dashboard_handler"
)

// Router mounts the User Dashboard pages and its JSON actions. dashboard is
// a *gin.RouterGroup already carrying AuthMiddleware (spec AC1.7) — this
// function doesn't apply auth itself, same division of responsibility as
// auth.Router not knowing about sessions.
func Router(dashboard *gin.RouterGroup, api *gin.RouterGroup, handler user_dashboard_handler.UserDashboardHandler) {
	dashboard.GET("", handler.Dashboard)
	dashboard.GET("/team", handler.TeamView)
	dashboard.GET("/overdue", handler.OverdueView)

	api.GET("/board", handler.GetBoard)
	api.GET("/team", handler.GetTeam)
	api.POST("/preference", handler.UpdatePreference)
	api.POST("/tickets/:issueKey/transition", handler.TransitionTicket)
	api.POST("/credential", handler.SaveCredential)
	api.DELETE("/credential", handler.DeleteCredential)
}
