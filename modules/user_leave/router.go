package user_leave

import (
	"github.com/gin-gonic/gin"

	"nudge/modules/user_leave/user_leave_handler"
)

// Router mounts the leave endpoints (Tech Spec "Papan Tiket & Cuti") on the
// dashboard API group, which already carries the auth middleware.
func Router(api *gin.RouterGroup, h *user_leave_handler.UserLeaveHandler) {
	api.GET("/leaves", h.List)
	api.POST("/leaves", h.Add)
	api.DELETE("/leaves/:date", h.Remove)
}
