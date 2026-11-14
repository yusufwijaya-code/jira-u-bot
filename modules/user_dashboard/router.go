package user_dashboard

import (
	"github.com/gin-gonic/gin"

	"nudge/modules/user_dashboard/user_dashboard_handler"
)

// Router mounts the User Dashboard pages and its JSON actions. dashboard is
// a *gin.RouterGroup already carrying AuthMiddleware (spec AC1.7) — this
// function doesn't apply auth itself, same division of responsibility as
// auth.Router not knowing about sessions.
func Router(api *gin.RouterGroup, handler user_dashboard_handler.UserDashboardHandler) {
	api.GET("/settings", handler.GetSettings)
	api.PUT("/settings/notification", handler.UpdateNotificationSettings)
	api.DELETE("/settings/notification", handler.ResetNotificationSettings)
	api.POST("/settings/mute", handler.Mute)
	api.DELETE("/settings/mute", handler.Unmute)
	api.GET("/boards", handler.ListBoards)
	api.GET("/boards/:boardId/avatar", handler.BoardAvatar)
	api.GET("/board", handler.GetBoard)
	api.GET("/team", handler.GetTeam)
	api.GET("/overdue", handler.GetOverdue)
	api.POST("/tickets/:issueKey/transition", handler.TransitionTicket)
	api.POST("/credential", handler.SaveCredential)
	api.DELETE("/credential", handler.DeleteCredential)
}
