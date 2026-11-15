package user_leave_handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/user_leave/user_leave_dto"
	"nudge/modules/user_leave/user_leave_service"
)

// UserLeaveHandler serves the session user's own leave days (Tech Spec
// "Papan Tiket & Cuti", US-2). Identity only ever comes from the session.
type UserLeaveHandler struct {
	svc user_leave_service.UserLeaveService
}

func NewUserLeaveHandler(svc user_leave_service.UserLeaveService) *UserLeaveHandler {
	return &UserLeaveHandler{svc: svc}
}

type addRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
	Note string `json:"note"`
}

func sessionEmail(c *gin.Context) string {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
}

func (h *UserLeaveHandler) List(c *gin.Context) {
	leaves, err := h.svc.List(c.Request.Context(), sessionEmail(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memuat cuti."})
		return
	}
	if leaves == nil {
		leaves = []user_leave_dto.Leave{}
	}
	c.JSON(http.StatusOK, gin.H{"leaves": leaves})
}

func (h *UserLeaveHandler) Add(c *gin.Context) {
	var req addRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.From == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tanggal mulai wajib diisi."})
		return
	}
	res, err := h.svc.Add(c.Request.Context(), sessionEmail(c), req.From, req.To, req.Note)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, res)
	case errors.Is(err, user_leave_service.ErrInvalidRange):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rentang tanggal tidak valid (maks 31 hari, paling lama 60 hari ke belakang, catatan maks 200 karakter)."})
	case errors.Is(err, user_leave_service.ErrNoWorkingDay):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Tidak ada hari kerja di rentang ini."})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menyimpan cuti."})
	}
}

func (h *UserLeaveHandler) Remove(c *gin.Context) {
	date := c.Param("date")
	recalculating, err := h.svc.Remove(c.Request.Context(), sessionEmail(c), date)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"deleted": date, "recalculating": recalculating})
	case errors.Is(err, user_leave_service.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Cuti tidak ditemukan."})
	case errors.Is(err, user_leave_service.ErrInvalidRange):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format tanggal salah."})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menghapus cuti."})
	}
}
