package issue_create_handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	svc "nudge/modules/issue_create/issue_create_service"
	"nudge/modules/jira_client/jira_client_service"
)

type Service interface {
	Status(ctx context.Context, email string) (svc.Status, error)
	Meta(ctx context.Context, email string, boardID int64) (svc.Meta, error)
	Users(ctx context.Context, email string, boardID int64, q string) ([]svc.User, error)
	Parent(ctx context.Context, email string, boardID int64, key string) (svc.Parent, error)
	CreateOne(ctx context.Context, email string, req svc.CreateRequest) (string, error)
	Normalize(ctx context.Context, email string, boardID int64, text string) (svc.Normalized, error)
	CreateSubtasks(ctx context.Context, email string, boardID int64, parentKey string, rows []svc.BulkRow) ([]svc.BulkResult, error)
}

type Handler struct {
	s           Service
	jiraBaseURL string
}

func New(s Service, jiraBaseURL string) *Handler {
	return &Handler{s: s, jiraBaseURL: strings.TrimRight(jiraBaseURL, "/")}
}

// Routes mounts /create/* on a group that already carries auth.
func Routes(g *gin.RouterGroup, h *Handler) {
	c := g.Group("/create")
	c.GET("/status", h.status)
	c.GET("/meta", h.meta)
	c.GET("/users", h.users)
	c.GET("/parent/:key", h.parent)
	c.POST("/issue", h.createOne)
	c.POST("/normalize", h.normalize)
	c.POST("/subtasks", h.subtasks)
}

func email(c *gin.Context) string {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
}

func boardID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Query("boardId"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "boardId wajib"})
		return 0, false
	}
	return id, true
}

func (h *Handler) fail(c *gin.Context, err error) {
	var v *svc.ValidationError
	var rl *svc.RateLimitError
	var rej *jira_client_service.RejectedError
	switch {
	case errors.Is(err, svc.ErrTokenRequired):
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "jira_token_required", "message": "Isi token Jira di Pengaturan dulu"})
	case errors.Is(err, svc.ErrBoardNotAllowed):
		c.JSON(http.StatusBadRequest, gin.H{"error": "board tidak terdaftar untuk kamu"})
	case errors.As(err, &v):
		c.JSON(http.StatusBadRequest, gin.H{"error": v.Message})
	case errors.Is(err, svc.ErrParentNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "parent tidak ditemukan"})
	case errors.Is(err, svc.ErrParentInvalid):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "parent harus tiket non-subtask di project board ini"})
	case errors.As(err, &rl):
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited", "scope": rl.Scope, "retryAfterSeconds": rl.RetryAfterSeconds})
	case errors.Is(err, svc.ErrAIUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai_unavailable"})
	case errors.As(err, &rej):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "jira_rejected", "fields": rej.Fields, "messages": rej.Messages})
	default:
		c.JSON(http.StatusBadGateway, gin.H{"error": "Jira/AI tidak merespons, coba lagi"})
	}
}

func (h *Handler) status(c *gin.Context) {
	st, err := h.s.Status(c.Request.Context(), email(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *Handler) meta(c *gin.Context) {
	id, ok := boardID(c)
	if !ok {
		return
	}
	m, err := h.s.Meta(c.Request.Context(), email(c), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *Handler) users(c *gin.Context) {
	id, ok := boardID(c)
	if !ok {
		return
	}
	u, err := h.s.Users(c.Request.Context(), email(c), id, c.Query("q"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": u})
}

func (h *Handler) parent(c *gin.Context) {
	id, ok := boardID(c)
	if !ok {
		return
	}
	p, err := h.s.Parent(c.Request.Context(), email(c), id, c.Param("key"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) createOne(c *gin.Context) {
	var req svc.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.BoardID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request tidak valid"})
		return
	}
	key, err := h.s.CreateOne(c.Request.Context(), email(c), req)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"key": key, "url": h.jiraBaseURL + "/browse/" + key})
}

func (h *Handler) normalize(c *gin.Context) {
	var req struct {
		BoardID int64  `json:"boardId"`
		Text    string `json:"text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.BoardID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request tidak valid"})
		return
	}
	n, err := h.s.Normalize(c.Request.Context(), email(c), req.BoardID, req.Text)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, n)
}

func (h *Handler) subtasks(c *gin.Context) {
	var req struct {
		BoardID   int64         `json:"boardId"`
		ParentKey string        `json:"parentKey"`
		Rows      []svc.BulkRow `json:"rows"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.BoardID <= 0 || strings.TrimSpace(req.ParentKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request tidak valid"})
		return
	}
	res, err := h.s.CreateSubtasks(c.Request.Context(), email(c), req.BoardID, req.ParentKey, req.Rows)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": res})
}
