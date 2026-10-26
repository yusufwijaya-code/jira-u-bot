package registration_handler

import "github.com/gin-gonic/gin"

type RegistrationHandler interface {
	ShowForm(c *gin.Context)
}
