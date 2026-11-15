// Package chat_app_handler serves POST /api/chat/events — the Nudge Chat
// App's HTTP endpoint (Tech Spec "Nudge sebagai Google Chat App", AC2.1, AC2.7).
package chat_app_handler

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/chat_app/chat_app_service"
)

const (
	// maxEventBytes caps the event body (read only after the JWT check).
	maxEventBytes = 1 << 20
	// rejectLogInterval: at most one "rejected event" log line per interval;
	// the next line reports how many were suppressed in between.
	rejectLogInterval = 10 * time.Second
)

type TokenVerifier interface {
	Verify(ctx context.Context, token string) error
}

type Handler struct {
	verifier TokenVerifier
	svc      chat_app_service.Service
	rejects  rejectLog
}

func New(verifier TokenVerifier, svc chat_app_service.Service) *Handler {
	return &Handler{verifier: verifier, svc: svc}
}

// rejectLog samples the log of rejected tokens so an unauthenticated
// caller can't flood the log.
type rejectLog struct {
	mu         sync.Mutex
	last       time.Time
	suppressed int
}

func (r *rejectLog) log(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if !r.last.IsZero() && now.Sub(r.last) < rejectLogInterval {
		r.suppressed++
		return
	}
	log.Printf("chat_app_handler: rejected event: %v (%d more rejected since the last log line)", err, r.suppressed)
	r.last, r.suppressed = now, 0
}

func (h *Handler) Events(c *gin.Context) {
	token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !ok || token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if err := h.verifier.Verify(c.Request.Context(), token); err != nil {
		h.rejects.log(err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxEventBytes)
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
