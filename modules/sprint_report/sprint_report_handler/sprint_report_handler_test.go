package sprint_report_handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"jira-u-bot/config"
	"jira-u-bot/modules/auth/auth_dto"
	"jira-u-bot/modules/auth/auth_middleware"
	"jira-u-bot/modules/sprint_report/sprint_report_dto"
)

func testCfg() *config.AppConfig {
	return &config.AppConfig{BasePath: "/jira-notif"}
}

type fakeSprintReportService struct {
	availableSprints []string
	report           sprint_report_dto.Report
	lastSprintNames  []string
	lastCap          float64
}

func (f *fakeSprintReportService) ListAvailableSprints(ctx context.Context) ([]string, error) {
	return f.availableSprints, nil
}
func (f *fakeSprintReportService) GenerateReport(ctx context.Context, sprintNames []string, capPercent float64) (sprint_report_dto.Report, error) {
	f.lastSprintNames = sprintNames
	f.lastCap = capPercent
	return f.report, nil
}

func newReportTestContext(target string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", target, nil)
	c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "dev@esb.co.id"})
	return w, c
}

func TestReport_ShowsSprintPickerWithoutSelection(t *testing.T) {
	svc := &fakeSprintReportService{availableSprints: []string{"Sprint 20", "Sprint 19"}}
	handler := NewSprintReportHandler(svc, testCfg())

	w, c := newReportTestContext("/dashboard/report")
	handler.Report(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Sprint 20") || !strings.Contains(body, "Sprint 19") {
		t.Error("body missing available sprint options")
	}
	if strings.Contains(body, "Aggregate Deviation") {
		t.Error("body should not show report summary cards when no sprint is selected")
	}
}

func TestReport_GeneratesReportWithSelectedSprintsAndDefaultCap(t *testing.T) {
	svc := &fakeSprintReportService{
		availableSprints: []string{"Sprint 20"},
		report: sprint_report_dto.Report{
			Rows:                      []sprint_report_dto.Row{{IssueKey: "OMS-1", Task: "Test", EstimatedHours: 7, ActualHours: 5.44, DeviationPercent: 22.29}},
			AggregateDeviationPercent: 22.29,
			AchievementPercent:        53.84,
			WithinCount:               0,
			TotalCount:                1,
		},
	}
	handler := NewSprintReportHandler(svc, testCfg())

	w, c := newReportTestContext("/dashboard/report?sprint=Sprint+20")
	handler.Report(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(svc.lastSprintNames) != 1 || svc.lastSprintNames[0] != "Sprint 20" {
		t.Errorf("GenerateReport called with sprintNames=%v, want [Sprint 20]", svc.lastSprintNames)
	}
	if svc.lastCap != 12 {
		t.Errorf("GenerateReport called with cap=%v, want default 12", svc.lastCap)
	}
	body := w.Body.String()
	if !strings.Contains(body, "OMS-1") || !strings.Contains(body, "Aggregate Deviation") {
		t.Error("body missing generated report content")
	}
}

func TestReport_RespectsCustomCapParam(t *testing.T) {
	svc := &fakeSprintReportService{availableSprints: []string{"Sprint 20"}}
	handler := NewSprintReportHandler(svc, testCfg())

	w, c := newReportTestContext("/dashboard/report?sprint=Sprint+20&cap=20")
	handler.Report(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if svc.lastCap != 20 {
		t.Errorf("GenerateReport called with cap=%v, want 20 (spec AC — ?cap= override)", svc.lastCap)
	}
}

func TestReport_MultipleSprintsSelected(t *testing.T) {
	svc := &fakeSprintReportService{availableSprints: []string{"Sprint 19", "Sprint 20"}}
	handler := NewSprintReportHandler(svc, testCfg())

	w, c := newReportTestContext("/dashboard/report?sprint=Sprint+19&sprint=Sprint+20")
	handler.Report(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(svc.lastSprintNames) != 2 {
		t.Errorf("GenerateReport called with sprintNames=%v, want 2 entries", svc.lastSprintNames)
	}
}
