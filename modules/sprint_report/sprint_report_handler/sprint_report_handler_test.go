package sprint_report_handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/sprint_report/sprint_report_dto"
)

type fakeSprintReportService struct {
	availableSprints    []string
	report              sprint_report_dto.Report
	lastSprintNames     []string
	lastCap             float64
	lastSelectedSprints []string
	lastCapPercent      float64
	lastListEmail       string
	lastGenerateEmail   string
	listErr             error
	generateErr         error
}

func (f *fakeSprintReportService) ListAvailableSprints(ctx context.Context, email string) ([]string, error) {
	f.lastListEmail = email
	return f.availableSprints, f.listErr
}
func (f *fakeSprintReportService) GenerateReport(ctx context.Context, email string, sprintNames []string, capPercent float64) (sprint_report_dto.Report, error) {
	f.lastGenerateEmail = email
	f.lastSprintNames = sprintNames
	f.lastSelectedSprints = sprintNames
	f.lastCap = capPercent
	f.lastCapPercent = capPercent
	return f.report, f.generateErr
}

func newReportTestContext(target string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", target, nil)
	c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "dev@esb.co.id"})
	return w, c
}

func TestGetReport_WithoutSprintParamReturnsNullReport(t *testing.T) {
	svc := &fakeSprintReportService{availableSprints: []string{"Sprint 42", "Sprint 43"}}
	handler := NewSprintReportHandler(svc)

	w, c := newReportTestContext("/api/dashboard/report")
	handler.GetReport(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"Sprint 42"`) {
		t.Errorf("body missing availableSprints: %s", body)
	}
	if !strings.Contains(body, `"report":null`) {
		t.Errorf("body = %q, want report:null when no sprint selected", body)
	}
}

func TestGetReport_WithSprintParamGeneratesReport(t *testing.T) {
	svc := &fakeSprintReportService{
		availableSprints: []string{"Sprint 42"},
		report:           sprint_report_dto.Report{SumEstimatedHours: 8, SumActualHours: 7.5},
	}
	handler := NewSprintReportHandler(svc)

	w, c := newReportTestContext("/api/dashboard/report?sprint=Sprint+42")
	handler.GetReport(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"sumEstimatedHours":8`) {
		t.Errorf("body missing generated report: %s", w.Body.String())
	}
	if svc.lastSelectedSprints == nil || svc.lastSelectedSprints[0] != "Sprint 42" {
		t.Errorf("GenerateReport called with %v, want [Sprint 42]", svc.lastSelectedSprints)
	}
}

func TestGetReport_InvalidCapFallsBackToDefault(t *testing.T) {
	svc := &fakeSprintReportService{availableSprints: []string{"Sprint 42"}, report: sprint_report_dto.Report{}}
	handler := NewSprintReportHandler(svc)

	w, c := newReportTestContext("/api/dashboard/report?sprint=Sprint+42&cap=notanumber")
	handler.GetReport(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if svc.lastCapPercent != defaultCapPercent {
		t.Errorf("GenerateReport called with cap=%v, want default %v", svc.lastCapPercent, defaultCapPercent)
	}
}

func TestGetReport_ListAvailableSprintsErrorReturns500(t *testing.T) {
	svc := &fakeSprintReportService{listErr: errors.New("jira down")}
	handler := NewSprintReportHandler(svc)

	w, c := newReportTestContext("/api/dashboard/report")
	handler.GetReport(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetReport_GenerateReportErrorReturns500(t *testing.T) {
	svc := &fakeSprintReportService{availableSprints: []string{"Sprint 42"}, generateErr: errors.New("jira down")}
	handler := NewSprintReportHandler(svc)

	w, c := newReportTestContext("/api/dashboard/report?sprint=Sprint+42")
	handler.GetReport(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetReport_UsesSessionEmailNotQueryParam(t *testing.T) {
	svc := &fakeSprintReportService{availableSprints: []string{"Sprint 42"}}
	handler := NewSprintReportHandler(svc)

	// A hostile ?email= must be ignored - identity only comes from the session.
	w, c := newReportTestContext("/api/dashboard/report?sprint=Sprint+42&email=victim@esb.co.id")
	handler.GetReport(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if svc.lastListEmail != "dev@esb.co.id" || svc.lastGenerateEmail != "dev@esb.co.id" {
		t.Errorf("service called with list=%q generate=%q, want the session's dev@esb.co.id", svc.lastListEmail, svc.lastGenerateEmail)
	}
}

// --- POST /report/backfill (Tech Spec "Active Sprint Tracking & Changelog Backfill") ---

type fakeBackfiller struct {
	email   string
	added   int
	updated int
	err     error
}

func (f *fakeBackfiller) RunForEmail(ctx context.Context, email string) (int, int, error) {
	f.email = email
	return f.added, f.updated, f.err
}

func TestBackfill_RunsForTheSessionUser(t *testing.T) {
	b := &fakeBackfiller{added: 8, updated: 3}
	h := NewSprintReportHandler(&fakeSprintReportService{}, WithBackfiller(b))

	w, c := newReportTestContext("/api/dashboard/report/backfill?email=victim@esb.co.id")
	c.Request.Method = http.MethodPost
	h.Backfill(c)

	if w.Code != http.StatusOK || w.Body.String() != `{"added":8,"updated":3}` {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if b.email != "dev@esb.co.id" {
		t.Errorf("backfilled for %q, want the session user", b.email)
	}
}

func TestBackfill_ErrorsAndMissingBackfiller(t *testing.T) {
	w, c := newReportTestContext("/api/dashboard/report/backfill")
	NewSprintReportHandler(&fakeSprintReportService{}, WithBackfiller(&fakeBackfiller{err: errors.New("jira down")})).Backfill(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("jira failure status = %d, want 500", w.Code)
	}

	w, c = newReportTestContext("/api/dashboard/report/backfill")
	NewSprintReportHandler(&fakeSprintReportService{}).Backfill(c)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("no backfiller status = %d, want 501", w.Code)
	}
}
