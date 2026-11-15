// Package chat_app_handler serves POST /api/chat/events — the Nudge Chat
// App's HTTP endpoint (Tech Spec "Nudge sebagai Google Chat App", AC2.1, AC2.7).
package chat_app_handler

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/chat_app/chat_app_service"
)

type TokenVerifier interface {
	Verify(ctx context.Context, token string) error
}

type Handler struct {
	verifier TokenVerifier
	svc      chat_app_service.Service
}

func New(verifier TokenVerifier, svc chat_app_service.Service) *Handler {
	return &Handler{verifier: verifier, svc: svc}
}

func (h *Handler) Events(c *gin.Context) {
	token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !ok || token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if err := h.verifier.Verify(c.Request.Context(), token); err != nil {
		log.Printf("chat_app_handler: rejected event: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var ev chat_app_dto.Event
	if err := c.ShouldBindJSON(&ev); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event"})
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("chat_app_handler: %s event panicked: %v", ev.Type, r)
			c.JSON(http.StatusOK, chat_app_dto.Response{Text: "Terjadi kesalahan, coba lagi."})
		}
	}()
	c.JSON(http.StatusOK, h.svc.Handle(c.Request.Context(), ev))
}
