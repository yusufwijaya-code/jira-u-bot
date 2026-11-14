package sprint_report_handler

import (
	"context"
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
	svc      sprint_report_service.SprintReportService
	backfill Backfiller
}

// Backfiller is the sprint_backfill_service capability behind Backfill.
type Backfiller interface {
	RunForEmail(ctx context.Context, email string) (added int, err error)
}

// Option configures the handler.
type Option func(*sprintReportHandlerImpl)

// WithBackfiller enables POST /report/backfill.
func WithBackfiller(b Backfiller) Option {
	return func(h *sprintReportHandlerImpl) { h.backfill = b }
}

func NewSprintReportHandler(svc sprint_report_service.SprintReportService, opts ...Option) SprintReportHandler {
	h := &sprintReportHandlerImpl{svc: svc}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *sprintReportHandlerImpl) Backfill(c *gin.Context) {
	if h.backfill == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "backfill tidak tersedia"})
		return
	}
	email := c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
	added, err := h.backfill.RunForEmail(c.Request.Context(), email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil data dari Jira"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"added": added})
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
