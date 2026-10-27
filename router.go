package main

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/chat_interactive"
	"jira-u-bot/modules/chat_interactive/chat_interactive_handler"
	"jira-u-bot/modules/chat_interactive/chat_interactive_middleware"
	"jira-u-bot/modules/registration"
	"jira-u-bot/modules/registration/registration_handler"
)

func SetupRouter(registrationHandler registration_handler.RegistrationHandler, chatInteractiveHandler chat_interactive_handler.ChatInteractiveHandler, chatEventsVerifier chat_interactive_middleware.TokenVerifier, chatEventsAudience string) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	registration.Router(r, registrationHandler)

	if chatEventsAudience != "" {
		chat_interactive.Router(r, chatEventsVerifier, chatEventsAudience, chatInteractiveHandler)
	}

	return r
}
