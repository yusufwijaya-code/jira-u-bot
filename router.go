package main

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/registration"
	"jira-u-bot/modules/registration/registration_handler"
)

func SetupRouter(registrationHandler registration_handler.RegistrationHandler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	registration.Router(r, registrationHandler)

	return r
}
