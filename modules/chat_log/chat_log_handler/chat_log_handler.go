// Package chat_log_handler serves the admin "Riwayat pesan" list.
package chat_log_handler

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/chat_log/chat_log_dto"
	"nudge/modules/user_setting/user_setting_dto"
)

type Lister interface {
	List(ctx context.Context, f chat_log_dto.Filter) (chat_log_dto.Page, error)
	Conversations(ctx context.Context) ([]chat_log_dto.Conversation, error)
}

var knownKinds = map[string]bool{
	chat_log_dto.KindReminder: true, chat_log_dto.KindMessage: true, chat_log_dto.KindReply: true,
	chat_log_dto.KindCommand: true, chat_log_dto.KindClick: true, chat_log_dto.KindAdded: true, chat_log_dto.KindRemoved: true,
}

// Conversations answers GET /api/dashboard/admin/chat-logs/conversations
// (admin only): one row per user, most recent first.
func (h *Handler) Conversations(c *gin.Context) {
	list, err := h.logs.Conversations(c.Request.Context())
	if err != nil {
		log.Printf("chat_log_handler: conversations failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat percakapan"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"conversations": list})
}

type Handler struct {
	logs Lister
}

func New(logs Lister) *Handler {
	return &Handler{logs: logs}
}

// List answers GET /api/dashboard/admin/chat-logs (admin only) with
// ?email=&from=YYYY-MM-DD&to=YYYY-MM-DD (WIB, both inclusive)&beforeID=&limit=.
func (h *Handler) List(c *gin.Context) {
	f := chat_log_dto.Filter{Email: strings.TrimSpace(c.Query("email"))}
	var err error
	if f.From, err = parseDay(c.Query("from")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from harus berformat YYYY-MM-DD"})
		return
	}
	to, err := parseDay(c.Query("to"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to harus berformat YYYY-MM-DD"})
		return
	}
	if !to.IsZero() {
		f.To = to.AddDate(0, 0, 1)
	}
	if v := c.Query("beforeID"); v != "" {
		if f.BeforeID, err = strconv.ParseInt(v, 10, 64); err != nil || f.BeforeID < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "beforeID tidak valid"})
			return
		}
	}
	if v := c.Query("afterID"); v != "" {
		if f.AfterID, err = strconv.ParseInt(v, 10, 64); err != nil || f.AfterID < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "afterID tidak valid"})
			return
		}
	}
	if v := c.Query("kind"); v != "" {
		for _, k := range strings.Split(v, ",") {
			if !knownKinds[k] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "kind tidak valid"})
				return
			}
			f.Kinds = append(f.Kinds, k)
		}
	}
	if v := c.Query("limit"); v != "" {
		if f.Limit, err = strconv.Atoi(v); err != nil || f.Limit < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit tidak valid"})
			return
		}
	}
	page, err := h.logs.List(c.Request.Context(), f)
	if err != nil {
		log.Printf("chat_log_handler: list failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat riwayat pesan"})
		return
	}
	c.JSON(http.StatusOK, page)
}

func parseDay(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.ParseInLocation("2006-01-02", s, user_setting_dto.Jakarta)
}
