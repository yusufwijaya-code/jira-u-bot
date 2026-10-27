package registration

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/registration/registration_handler"
)

func Router(r *gin.Engine, handler registration_handler.RegistrationHandler) {
	group := r.Group("/esb-jira-notif")
	group.GET("/register", handler.ShowForm)
	group.POST("/register", handler.Submit)
}
