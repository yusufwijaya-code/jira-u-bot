package sprint_report_handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/sprint_report/sprint_report_service"
)

// defaultCapPercent is spec Open Question #7 — the deviation cap, matching
// the PDF example's own 12% target, overridable per-report via ?cap=.
const defaultCapPercent float64 = 12

type sprintReportHandlerImpl struct {
	svc sprint_report_service.SprintReportService
}

func NewSprintReportHandler(svc sprint_report_service.SprintReportService) SprintReportHandler {
	return &sprintReportHandlerImpl{svc: svc}
}

func (h *sprintReportHandlerImpl) GetReport(c *gin.Context) {
	ctx := c.Request.Context()
	// The caller's identity always comes from the session - never from a
	// query param - so a report can only ever cover the caller's own tickets.
	email := c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email

	availableSprints, err := h.svc.ListAvailableSprints(ctx, email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load sprint list"})
		return
	}

	selected := c.QueryArray("sprint")

	capPercent := defaultCapPercent
	if raw := c.Query("cap"); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
			capPercent = parsed
		}
	}

	var report interface{}
	if len(selected) > 0 {
		generated, err := h.svc.GenerateReport(ctx, email, selected, capPercent)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate report"})
			return
		}
		report = generated
	}

	c.JSON(http.StatusOK, gin.H{
		"availableSprints": availableSprints,
		"capPercent":       capPercent,
		"report":           report,
	})
}
