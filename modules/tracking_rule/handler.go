package tracking_rule

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
)

// API is what the handler needs from Service.
type API interface {
	List(ctx context.Context) ([]RuleView, error)
	Lookup(ctx context.Context, projectKey string, fresh bool) (Lookup, error)
	Create(ctx context.Context, in RuleInput, editor string) (RuleView, error)
	Update(ctx context.Context, projectKey string, in RuleInput, editor string) (RuleView, error)
	SetActive(ctx context.Context, projectKey string, active bool, editor string) (RuleView, error)
	Recalculate() error
	BoardStatuses(ctx context.Context, boardID int64) ([]string, error)
	CreateFields(ctx context.Context, projectKey string, fresh bool) ([]CreateFieldType, error)
}

type Handler struct{ api API }

func NewHandler(api API) *Handler { return &Handler{api: api} }

// AdminRoutes mounts Admin › Aturan Board on a group already behind
// AuthMiddlewareAPI + AdminMiddleware.
func AdminRoutes(g *gin.RouterGroup, h *Handler) {
	g.GET("/tracking-rules", h.list)
	// Static segment: gin matches it before the :projectKey wildcard.
	g.GET("/tracking-rules/jira/create-fields", h.createFields)
	g.GET("/tracking-rules/jira/:projectKey", h.lookup)
	g.POST("/tracking-rules", h.create)
	g.POST("/tracking-rules/recalculate", h.recalculate)
	g.PUT("/tracking-rules/:projectKey", h.update)
	g.PUT("/tracking-rules/:projectKey/active", h.setActive)
}

// UserRoutes mounts the board workflow statuses (Pengaturan) on a group
// already behind AuthMiddlewareAPI. :boardId matches /boards/:boardId/avatar.
func UserRoutes(g *gin.RouterGroup, h *Handler) {
	g.GET("/boards/:boardId/statuses", h.boardStatuses)
}

const maxBodyBytes = 64 << 10

func editor(c *gin.Context) string {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
}

func displayKey(raw string) string {
	key, _ := NormalizeProjectKey(raw)
	return key
}

func fail(c *gin.Context, err error, key string) {
	var v *ValidationError
	switch {
	case errors.As(err, &v):
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": "Periksa isian yang ditandai", "fields": v.Fields})
	case errors.Is(err, ErrInvalidProjectKey):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_project_key", "message": "Format project key tidak valid"})
	case errors.Is(err, ErrProjectNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "project_not_found", "message": fmt.Sprintf("Project %s tidak ditemukan di Jira", key)})
	case errors.Is(err, ErrRuleNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "rule_not_found", "message": "Aturan board tidak ditemukan"})
	case errors.Is(err, ErrRuleExists):
		c.JSON(http.StatusConflict, gin.H{"error": "rule_exists", "message": fmt.Sprintf("Project %s sudah punya aturan — ubah lewat Edit", key)})
	case errors.Is(err, ErrLastActiveRule):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "last_active_rule", "message": "Minimal satu board harus aktif"})
	case errors.Is(err, ErrAlreadyRunning):
		c.JSON(http.StatusConflict, gin.H{"error": "already_running", "message": "Hitung ulang masih berjalan"})
	case errors.Is(err, ErrBoardNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "board_not_found", "message": "Board tidak dikenal"})
	case errors.Is(err, ErrJiraUnavailable):
		c.JSON(http.StatusBadGateway, gin.H{"error": "jira_unavailable", "message": "Jira tidak merespons, coba lagi"})
	default:
		log.Printf("tracking_rule: request failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "Terjadi kesalahan, coba lagi"})
	}
}

func bindInput(c *gin.Context) (RuleInput, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	var in RuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": "Format data tidak valid"})
		return in, false
	}
	return in, true
}

func (h *Handler) list(c *gin.Context) {
	rules, err := h.api.List(c.Request.Context())
	if err != nil {
		fail(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

func (h *Handler) lookup(c *gin.Context) {
	key := c.Param("projectKey")
	// ?fresh=1 bypasses the 10-minute cache ("Muat ulang dari Jira").
	l, err := h.api.Lookup(c.Request.Context(), key, c.Query("fresh") == "1")
	if err != nil {
		fail(c, err, displayKey(key))
		return
	}
	c.JSON(http.StatusOK, l)
}

func (h *Handler) createFields(c *gin.Context) {
	key := c.Query("projectKey")
	types, err := h.api.CreateFields(c.Request.Context(), key, c.Query("fresh") == "1")
	if err != nil {
		fail(c, err, displayKey(key))
		return
	}
	c.JSON(http.StatusOK, gin.H{"types": types})
}

func (h *Handler) create(c *gin.Context) {
	in, ok := bindInput(c)
	if !ok {
		return
	}
	rule, err := h.api.Create(c.Request.Context(), in, editor(c))
	if err != nil {
		fail(c, err, displayKey(in.ProjectKey))
		return
	}
	c.JSON(http.StatusCreated, rule)
}

func (h *Handler) update(c *gin.Context) {
	key := c.Param("projectKey")
	in, ok := bindInput(c)
	if !ok {
		return
	}
	rule, err := h.api.Update(c.Request.Context(), key, in, editor(c))
	if err != nil {
		fail(c, err, displayKey(key))
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *Handler) setActive(c *gin.Context) {
	var body struct {
		Active *bool `json:"active"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Active == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": "Isi active: true atau false"})
		return
	}
	key := c.Param("projectKey")
	rule, err := h.api.SetActive(c.Request.Context(), key, *body.Active, editor(c))
	if err != nil {
		fail(c, err, displayKey(key))
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *Handler) recalculate(c *gin.Context) {
	if err := h.api.Recalculate(); err != nil {
		fail(c, err, "")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"started": true})
}

func (h *Handler) boardStatuses(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("boardId"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_board_id", "message": "Board tidak valid"})
		return
	}
	statuses, err := h.api.BoardStatuses(c.Request.Context(), id)
	if err != nil {
		fail(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"statuses": statuses})
}
