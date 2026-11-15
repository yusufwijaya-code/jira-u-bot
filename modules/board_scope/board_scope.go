// Package board_scope stores the board the user picked in the web navbar
// (ms_user.selectedBoardID, 0 = "Semua board"), so the chat commands show
// the same board as the web.
package board_scope

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
)

type Repository struct{ db *sql.DB }

func New(db *sql.DB) *Repository { return &Repository{db: db} }

// Get returns the user's board (0 = every board, also for an unknown user).
func (r *Repository) Get(ctx context.Context, email string) (int64, error) {
	var id sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT selectedBoardID FROM ms_user WHERE LOWER(email) = ?`, strings.ToLower(email)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return id.Int64, nil
}

// Set stores the user's board (0 = every board).
func (r *Repository) Set(ctx context.Context, email string, boardID int64) error {
	var v any
	if boardID > 0 {
		v = boardID
	}
	_, err := r.db.ExecContext(ctx, `UPDATE ms_user SET selectedBoardID = ? WHERE LOWER(email) = ?`, v, strings.ToLower(email))
	return err
}

// Store is what the handler needs.
type Store interface {
	Get(ctx context.Context, email string) (int64, error)
	Set(ctx context.Context, email string, boardID int64) error
}

// Handler serves GET/PUT /api/dashboard/board-scope for the session user.
type Handler struct{ store Store }

func NewHandler(store Store) *Handler { return &Handler{store: store} }

func email(c *gin.Context) string {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
}

// response: boardId null = "Semua board".
func response(id int64) gin.H {
	if id <= 0 {
		return gin.H{"boardId": nil}
	}
	return gin.H{"boardId": id}
}

func (h *Handler) Get(c *gin.Context) {
	id, err := h.store.Get(c.Request.Context(), email(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load board"})
		return
	}
	c.JSON(http.StatusOK, response(id))
}

func (h *Handler) Put(c *gin.Context) {
	var req struct {
		BoardID *int64 `json:"boardId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.BoardID != nil && *req.BoardID < 0) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid boardId"})
		return
	}
	var id int64
	if req.BoardID != nil {
		id = *req.BoardID
	}
	if err := h.store.Set(c.Request.Context(), email(c), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save board"})
		return
	}
	c.JSON(http.StatusOK, response(id))
}
