package admin_dashboard_handler

import (
	"errors"
	"html/template"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/admin_dashboard/admin_dashboard_dto"
	"jira-u-bot/modules/admin_dashboard/admin_dashboard_service"
	admintemplates "jira-u-bot/modules/admin_dashboard/templates"
	"jira-u-bot/modules/auth/auth_dto"
	"jira-u-bot/modules/auth/auth_middleware"
)

type adminPageData struct {
	Email string
	Users []admin_dashboard_dto.UserRow
}

type adminDashboardHandlerImpl struct {
	svc      admin_dashboard_service.AdminDashboardService
	template *template.Template
}

func NewAdminDashboardHandler(svc admin_dashboard_service.AdminDashboardService) AdminDashboardHandler {
	return &adminDashboardHandlerImpl{
		svc:      svc,
		template: template.Must(template.ParseFS(admintemplates.FS, "admin.html")),
	}
}

func currentUser(c *gin.Context) auth_dto.Session {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session)
}

func (h *adminDashboardHandlerImpl) UserList(c *gin.Context) {
	users, err := h.svc.ListUsers(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "Gagal memuat daftar user, coba lagi")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	h.template.Execute(c.Writer, adminPageData{Email: currentUser(c).Email, Users: users})
}

func userIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("userID"), 10, 64)
	return id, err == nil
}

func (h *adminDashboardHandlerImpl) handleServiceErr(c *gin.Context, err error) {
	if errors.Is(err, admin_dashboard_service.ErrCannotModifySelf) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tidak bisa mengubah/menghapus akun sendiri"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memproses, coba lagi"})
}

type toggleActiveRequest struct {
	Active bool `json:"active"`
}

func (h *adminDashboardHandlerImpl) ToggleActive(c *gin.Context) {
	targetUserID, ok := userIDParam(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid userID"})
		return
	}
	var req toggleActiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := h.svc.SetActive(c.Request.Context(), currentUser(c).UserID, targetUserID, req.Active); err != nil {
		h.handleServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type editJiraAccountIDRequest struct {
	JiraAccountID string `json:"jiraAccountId"`
}

func (h *adminDashboardHandlerImpl) EditJiraAccountID(c *gin.Context) {
	targetUserID, ok := userIDParam(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid userID"})
		return
	}
	var req editJiraAccountIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := h.svc.UpdateJiraAccountID(c.Request.Context(), targetUserID, req.JiraAccountID); err != nil {
		h.handleServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *adminDashboardHandlerImpl) DeleteUser(c *gin.Context) {
	targetUserID, ok := userIDParam(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid userID"})
		return
	}

	if err := h.svc.DeleteUser(c.Request.Context(), currentUser(c).UserID, targetUserID); err != nil {
		h.handleServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *adminDashboardHandlerImpl) DeleteUserCredential(c *gin.Context) {
	targetUserID, ok := userIDParam(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid userID"})
		return
	}

	if err := h.svc.DeleteUserCredential(c.Request.Context(), targetUserID); err != nil {
		h.handleServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
