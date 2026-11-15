// Package chat_app_activation serves the dashboard side of the Nudge Chat
// App (Tech Spec "Nudge sebagai Google Chat App", US-3, US-7): activation
// status, the sender choice, and the one-click activation round-trip.
package chat_app_activation

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/chat_app/chat_app_client"
	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/chat_delivery"
	"nudge/modules/user_dashboard/user_dashboard_handler"
)

const (
	stateCookieName   = "chat_app_state"
	stateCookieMaxAge = 10 * 60 // AC3.2: 10 minutes
	confirmText       = "✅ Mulai sekarang notifikasi Nudge dikirim dari sini."
	notifyTimeout     = 10 * time.Second
)

type StateStore interface {
	FindByEmail(ctx context.Context, email string) (chat_app_dto.State, bool, error)
	SetSpace(ctx context.Context, email, spaceID, chatUserID string) error
	SetSender(ctx context.Context, email string, sender *string) error
}

// SpaceSetup installs the app for the user owning the access token.
type SpaceSetup func(ctx context.Context, userAccessToken string) (string, error)

type Notifier interface {
	SendPlainTextDM(ctx context.Context, email, text string) error
}

type Deps struct {
	States StateStore
	// OAuth is nil when one-click activation is off.
	OAuth         OAuth
	SetupDM       SpaceSetup
	Notify        Notifier
	AppEnabled    bool
	OneClick      bool
	OperatorEmail string
	OperatorName  string
	// AppURL is FrontendURL + BasePath; CookiePath is BasePath.
	AppURL     string
	CookiePath string
}

type Handler struct {
	d Deps
}

func New(d Deps) *Handler {
	if d.SetupDM == nil {
		d.SetupDM = chat_app_client.SetupBotDM
	}
	return &Handler{d: d}
}

type StatusResponse struct {
	Enabled         bool       `json:"enabled"`
	Activated       bool       `json:"activated"`
	ActivatedDate   *time.Time `json:"activatedDate"`
	OneClick        bool       `json:"oneClick"`
	Sender          *string    `json:"sender"`
	EffectiveSender string     `json:"effectiveSender"`
	OperatorAccount bool       `json:"operatorAccount"`
	OperatorName    string     `json:"operatorName"`
}

func (h *Handler) status(ctx context.Context, email string) (StatusResponse, error) {
	s, _, err := h.d.States.FindByEmail(ctx, email)
	if err != nil {
		return StatusResponse{}, err
	}
	isOp := chat_delivery.IsOperator(email, h.d.OperatorEmail)
	return StatusResponse{
		Enabled:         h.d.AppEnabled,
		Activated:       s.Activated(),
		ActivatedDate:   s.ActivatedDate,
		OneClick:        h.d.AppEnabled && h.d.OneClick && h.d.OAuth != nil,
		Sender:          s.Sender,
		EffectiveSender: chat_delivery.EffectiveSender(s, isOp, h.d.AppEnabled),
		OperatorAccount: isOp,
		OperatorName:    h.d.OperatorName,
	}, nil
}

func (h *Handler) Status(resolve user_dashboard_handler.TargetResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		target, ok := resolve(c)
		if !ok {
			return
		}
		res, err := h.status(c.Request.Context(), target.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat status Nudge App"})
			return
		}
		c.JSON(http.StatusOK, res)
	}
}

type senderRequest struct {
	Sender string `json:"sender"`
}

func (h *Handler) SetSender(resolve user_dashboard_handler.TargetResolver, onChange user_dashboard_handler.ChangeListener) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in senderRequest
		if err := c.ShouldBindJSON(&in); err != nil || !chat_app_dto.ValidSender(in.Sender) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sender harus nudge atau operator"})
			return
		}
		target, ok := resolve(c)
		if !ok {
			return
		}
		ctx := c.Request.Context()
		if in.Sender == chat_app_dto.SenderOperator && chat_delivery.IsOperator(target.Email, h.d.OperatorEmail) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Akun ini hanya bisa memakai Nudge"})
			return
		}
		if in.Sender == chat_app_dto.SenderNudge && !h.d.AppEnabled {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "App Nudge belum dikonfigurasi di server"})
			return
		}
		state, _, err := h.d.States.FindByEmail(ctx, target.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menyimpan pengirim pesan"})
			return
		}
		if in.Sender == chat_app_dto.SenderNudge && !state.Activated() {
			c.JSON(http.StatusConflict, gin.H{"error": "chat_app_not_activated"})
			return
		}
		sender := in.Sender
		if err := h.d.States.SetSender(ctx, target.Email, &sender); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menyimpan pengirim pesan"})
			return
		}
		if onChange != nil {
			onChange(c, target, user_dashboard_handler.SettingsChange{Kind: user_dashboard_handler.ChangeChatSender})
		}
		h.confirm(target.Email)
		res, err := h.status(ctx, target.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat status Nudge App"})
			return
		}
		c.JSON(http.StatusOK, res)
	}
}

// confirm DMs the user through the sender they now use (AC7.5) — detached
// and best-effort.
func (h *Handler) confirm(email string) {
	if h.d.Notify == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		defer cancel()
		if err := h.d.Notify.SendPlainTextDM(ctx, email, confirmText); err != nil {
			log.Printf("chat_app_activation: confirmation DM to %s failed: %v", email, err)
		}
	}()
}

func currentUser(c *gin.Context) auth_dto.Session {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session)
}

func (h *Handler) cookiePath() string {
	if h.d.CookiePath == "" {
		return "/"
	}
	return h.d.CookiePath
}

// Activate starts the one-click activation (AC3.2). Mounted behind
// AuthMiddlewareAPI.
func (h *Handler) Activate(c *gin.Context) {
	then := ""
	if c.Query("then") == "sender" {
		then = "sender"
	}
	if h.d.OAuth == nil || !h.d.AppEnabled {
		h.finish(c, then, "failed")
		return
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		h.finish(c, then, "failed")
		return
	}
	state := base64.RawURLEncoding.EncodeToString(b)
	c.SetSameSite(http.SameSiteLaxMode)
	// Raw http.SetCookie: gin.SetCookie would percent-encode the "|".
	http.SetCookie(c.Writer, &http.Cookie{
		Name: stateCookieName, Value: state + "|" + then, MaxAge: stateCookieMaxAge,
		Path: h.cookiePath(), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	c.Redirect(http.StatusFound, h.d.OAuth.AuthCodeURL(state, currentUser(c).Email))
}

// Callback finishes it (AC3.3, AC3.4). The user's token is used once and
// dropped.
func (h *Handler) Callback(c *gin.Context) {
	raw, _ := c.Cookie(stateCookieName)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(stateCookieName, "", -1, h.cookiePath(), "", true, true)
	state, then, _ := strings.Cut(raw, "|")

	if raw == "" || state == "" || state != c.Query("state") {
		h.finish(c, then, "failed")
		return
	}
	if e := c.Query("error"); e != "" {
		h.finish(c, then, consentErrorCode(e))
		return
	}
	if h.d.OAuth == nil {
		h.finish(c, then, "failed")
		return
	}
	ctx := c.Request.Context()
	session := currentUser(c)
	token, email, err := h.d.OAuth.Exchange(ctx, c.Query("code"))
	if err != nil {
		log.Printf("chat_app_activation: exchange for %s failed: %v", session.Email, err)
		h.finish(c, then, "failed")
		return
	}
	if !strings.EqualFold(email, session.Email) {
		h.finish(c, then, "mismatch")
		return
	}
	space, err := h.d.SetupDM(ctx, token)
	if err != nil {
		log.Printf("chat_app_activation: spaces:setup for %s failed: %v", session.Email, err)
		if chat_app_client.IsGone(err) {
			h.finish(c, then, "not_allowed")
		} else {
			h.finish(c, then, "failed")
		}
		return
	}
	if err := h.d.States.SetSpace(ctx, session.Email, space, ""); err != nil {
		h.finish(c, then, "failed")
		return
	}
	if then == "sender" {
		nudge := chat_app_dto.SenderNudge
		if err := h.d.States.SetSender(ctx, session.Email, &nudge); err != nil {
			log.Printf("chat_app_activation: saving sender of %s failed: %v", session.Email, err)
		}
	}
	h.finish(c, then, "activated")
}

// consentErrorCode maps the OAuth "error" parameter to a chatApp code (AC3.4).
func consentErrorCode(e string) string {
	switch e {
	case "access_denied":
		return "cancelled"
	case "admin_policy_enforced", "org_internal", "disallowed_useragent":
		return "blocked"
	}
	return "failed"
}

func (h *Handler) finish(c *gin.Context, then, code string) {
	path := "/dashboard"
	if then == "sender" {
		path = "/dashboard/settings"
	}
	c.Redirect(http.StatusFound, h.d.AppURL+path+"?chatApp="+code)
}
