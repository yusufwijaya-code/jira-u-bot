// Package user_avatar_handler serves Jira profile pictures (Tech Spec
// "Admin Panel Revamp" — user avatars).
package user_avatar_handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/user_avatar/user_avatar_service"
)

type UserAvatarHandler struct {
	svc user_avatar_service.UserAvatarService
}

func NewUserAvatarHandler(svc user_avatar_service.UserAvatarService) *UserAvatarHandler {
	return &UserAvatarHandler{svc: svc}
}

// Mine serves the logged-in user's own picture (GET /api/dashboard/me/avatar).
func (h *UserAvatarHandler) Mine(c *gin.Context) {
	email := c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
	h.serve(c, func(ctx context.Context) ([]byte, string, error) { return h.svc.ByEmail(ctx, email) })
}

// ForUser serves any registered user's picture — mounted only behind
// AdminMiddleware (GET /api/dashboard/admin/users/:userID/avatar).
func (h *UserAvatarHandler) ForUser(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("userID"), 10, 64)
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid userID"})
		return
	}
	h.serve(c, func(ctx context.Context) ([]byte, string, error) { return h.svc.ByUserID(ctx, userID) })
}

func (h *UserAvatarHandler) serve(c *gin.Context, load func(ctx context.Context) ([]byte, string, error)) {
	body, contentType, err := load(c.Request.Context())
	if errors.Is(err, user_avatar_service.ErrNoAvatar) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err != nil {
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
	c.Header("Cache-Control", "private, max-age=86400")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, contentType, body)
}
