package registration

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/registration/registration_handler"
)

func Router(r *gin.Engine, handler registration_handler.RegistrationHandler) {
	r.GET("/register", handler.ShowForm)
	r.POST("/register", handler.Submit)
}
