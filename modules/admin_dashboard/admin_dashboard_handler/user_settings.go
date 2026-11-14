package admin_dashboard_handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"nudge/modules/admin_dashboard/admin_dashboard_dto"
	"nudge/modules/admin_dashboard/admin_dashboard_service"
	"nudge/modules/user_dashboard/user_dashboard_handler"
)

type userRow = admin_dashboard_dto.UserRow

// AdminTarget resolves /users/:userID to the user whose settings an admin
// is editing (Tech Spec "Admin Panel Revamp"): 400 for a bad id, 404 for an
// unknown user. Mounted only behind AdminMiddleware.
func AdminTarget(svc admin_dashboard_service.AdminDashboardService) user_dashboard_handler.TargetResolver {
	return func(c *gin.Context) (user_dashboard_handler.SettingsTarget, bool) {
		user, ok := loadUser(c, svc)
		if !ok {
			return user_dashboard_handler.SettingsTarget{}, false
		}
		return user_dashboard_handler.SettingsTarget{UserID: user.UserID, Email: user.Email}, true
	}
}

func (h *adminDashboardHandlerImpl) GetUser(c *gin.Context) {
	user, ok := loadUser(c, h.svc)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, user)
}

// loadUser reads :userID and loads that user, writing the error response
// itself when it returns false.
func loadUser(c *gin.Context, svc admin_dashboard_service.AdminDashboardService) (userRow, bool) {
	id, ok := userIDParam(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid userID"})
		return userRow{}, false
	}
	user, err := svc.GetUser(c.Request.Context(), id)
	if errors.Is(err, admin_dashboard_service.ErrUserNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user tidak ditemukan"})
		return userRow{}, false
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat user, coba lagi"})
		return userRow{}, false
	}
	return user, true
}
