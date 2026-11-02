package sprint_report_handler

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"jira-u-bot/config"
	"jira-u-bot/modules/auth/auth_dto"
	"jira-u-bot/modules/auth/auth_middleware"
	"jira-u-bot/modules/sprint_report/sprint_report_dto"
	"jira-u-bot/modules/sprint_report/sprint_report_service"
	sprintreporttemplates "jira-u-bot/modules/sprint_report/templates"
)

var templateFuncs = template.FuncMap{
	"fmt2": func(v float64) string { return fmt.Sprintf("%.2f", v) },
}

// defaultCapPercent is spec Open Question #7 — the deviation cap, matching
// the PDF example's own 12% target, overridable per-report via ?cap=.
const defaultCapPercent float64 = 12

type reportPageData struct {
	BasePath         string
	Email            string
	AvailableSprints []string
	SelectedSprints  map[string]bool
	HasSelection     bool
	CapPercent       float64
	Report           sprint_report_dto.Report
}

type sprintReportHandlerImpl struct {
	svc      sprint_report_service.SprintReportService
	basePath string
	template *template.Template
}

func NewSprintReportHandler(svc sprint_report_service.SprintReportService, cfg *config.AppConfig) SprintReportHandler {
	return &sprintReportHandlerImpl{
		svc:      svc,
		basePath: cfg.BasePath,
		template: template.Must(template.New("report.html").Funcs(templateFuncs).ParseFS(sprintreporttemplates.FS, "report.html")),
	}
}

func (h *sprintReportHandlerImpl) Report(c *gin.Context) {
	ctx := c.Request.Context()
	user := c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session)

	availableSprints, err := h.svc.ListAvailableSprints(ctx)
	if err != nil {
		c.String(http.StatusInternalServerError, "Gagal memuat daftar sprint, coba lagi")
		return
	}

	selected := c.QueryArray("sprint")
	selectedSet := make(map[string]bool, len(selected))
	for _, s := range selected {
		selectedSet[s] = true
	}

	capPercent := defaultCapPercent
	if raw := c.Query("cap"); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
			capPercent = parsed
		}
	}

	data := reportPageData{
		BasePath:         h.basePath,
		Email:            user.Email,
		AvailableSprints: availableSprints,
		SelectedSprints:  selectedSet,
		HasSelection:     len(selected) > 0,
		CapPercent:       capPercent,
	}

	if data.HasSelection {
		report, err := h.svc.GenerateReport(ctx, selected, capPercent)
		if err != nil {
			c.String(http.StatusInternalServerError, "Gagal membuat report, coba lagi")
			return
		}
		data.Report = report
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	h.template.Execute(c.Writer, data)
}

func (h *sprintReportHandlerImpl) GetReport(c *gin.Context) {
	ctx := c.Request.Context()

	availableSprints, err := h.svc.ListAvailableSprints(ctx)
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
		generated, err := h.svc.GenerateReport(ctx, selected, capPercent)
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
