package chat_interactive_handler

import "github.com/gin-gonic/gin"

type ChatInteractiveHandler interface {
	HandleEvent(c *gin.Context)
}
