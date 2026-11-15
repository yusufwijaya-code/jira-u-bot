// Package chat_log_handler serves the admin "Riwayat pesan" list.
package chat_log_handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"nudge/modules/chat_delivery"
	"nudge/modules/chat_log/chat_log_dto"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/user_setting/user_setting_dto"
)

type Lister interface {
	List(ctx context.Context, f chat_log_dto.Filter) (chat_log_dto.Page, error)
	Conversations(ctx context.Context) ([]chat_log_dto.Conversation, error)
}

// Sender delivers an admin's message through the recipient's sender.
type Sender interface {
	SendAdminMessage(ctx context.Context, email, text string) error
}

// UserFinder limits sending to registered users.
type UserFinder interface {
	FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error)
}

// MaxMessageRunes caps an admin message (Google Chat allows ~4096 chars).
const MaxMessageRunes = 4000

var knownKinds = map[string]bool{
	chat_log_dto.KindReminder: true, chat_log_dto.KindMessage: true, chat_log_dto.KindReply: true,
	chat_log_dto.KindCommand: true, chat_log_dto.KindClick: true, chat_log_dto.KindAdded: true, chat_log_dto.KindRemoved: true,
	chat_log_dto.KindAdmin: true,
}

type sendRequest struct {
	Email string `json:"email"`
	Text  string `json:"text"`
}

// Send answers POST /api/dashboard/admin/chat-logs/send (admin only): it
// DMs one registered user through their sender and returns
// {"status": "sent" | "skipped"}; skipped = no sender can reach them.
func (h *Handler) Send(c *gin.Context) {
	if h.send == nil || h.users == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pengiriman pesan tidak tersedia"})
		return
	}
	var req sendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format tidak valid"})
		return
	}
	email, text := strings.TrimSpace(req.Email), strings.TrimSpace(req.Text)
	if email == "" || text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user dan pesan wajib diisi"})
		return
	}
	if utf8.RuneCountInString(text) > MaxMessageRunes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pesan terlalu panjang"})
		return
	}
	u, found, err := h.users.FindUserByEmail(c.Request.Context(), email)
	switch {
	case err != nil:
		log.Printf("chat_log_handler: FindUserByEmail(%s) failed: %v", email, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memeriksa user"})
		return
	case !found:
		c.JSON(http.StatusNotFound, gin.H{"error": "user tidak terdaftar di Nudge"})
		return
	case !u.FlagActive:
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "user sedang nonaktif"})
		return
	}
	err = h.send.SendAdminMessage(c.Request.Context(), u.Email, text)
	switch {
	case errors.Is(err, chat_delivery.ErrRecipientNotActivated):
		c.JSON(http.StatusOK, gin.H{"status": chat_log_dto.StatusSkipped})
	case err != nil:
		log.Printf("chat_log_handler: admin message to %s failed: %v", u.Email, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "pesan gagal terkirim"})
	default:
		c.JSON(http.StatusOK, gin.H{"status": chat_log_dto.StatusSent})
	}
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
	logs  Lister
	send  Sender
	users UserFinder
}

// New builds the handler; send/users may be nil (sending then answers 503).
func New(logs Lister, send Sender, users UserFinder) *Handler {
	return &Handler{logs: logs, send: send, users: users}
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
