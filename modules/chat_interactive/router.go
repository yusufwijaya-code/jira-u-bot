package chat_interactive

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/chat_interactive/chat_interactive_handler"
	"jira-u-bot/modules/chat_interactive/chat_interactive_middleware"
)

// Router mounts the interactive bot's single endpoint. Mirrors
// modules/registration/router.go's group pattern.
func Router(r *gin.Engine, verifier chat_interactive_middleware.TokenVerifier, audience string, handler chat_interactive_handler.ChatInteractiveHandler) {
	group := r.Group("/jira-notif")
	group.POST("/chat-events", chat_interactive_middleware.VerifyChatRequest(verifier, audience), handler.HandleEvent)
}
