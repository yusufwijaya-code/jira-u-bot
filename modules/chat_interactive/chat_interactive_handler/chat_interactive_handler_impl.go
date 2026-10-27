package chat_interactive_handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/chat_interactive/chat_interactive_dto"
	"jira-u-bot/modules/chat_interactive/chat_interactive_service"
)

type chatInteractiveHandlerImpl struct {
	service chat_interactive_service.ChatInteractiveService
}

func NewChatInteractiveHandler(service chat_interactive_service.ChatInteractiveService) ChatInteractiveHandler {
	return &chatInteractiveHandlerImpl{service: service}
}

func (h *chatInteractiveHandlerImpl) HandleEvent(c *gin.Context) {
	var event chat_interactive_dto.ChatEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	text := h.service.RouteCommand(c.Request.Context(), event)
	c.JSON(http.StatusOK, gin.H{"text": text})
}
