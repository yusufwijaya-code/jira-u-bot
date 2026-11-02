package user_dashboard_handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"jira-u-bot/config"
	"jira-u-bot/modules/auth/auth_dto"
	"jira-u-bot/modules/auth/auth_middleware"
	"jira-u-bot/modules/jira_client/jira_client_dto"
	"jira-u-bot/modules/jira_credential/jira_credential_service"
	"jira-u-bot/modules/user_dashboard/user_dashboard_dto"
)

func testCfg() *config.AppConfig {
	return &config.AppConfig{BasePath: "/jira-notif"}
}

type fakeUserDashboardService struct {
	board            user_dashboard_dto.Board
	boardErr         error
	teamView         user_dashboard_dto.TeamView
	teamViewErr      error
	overdueItems     []user_dashboard_dto.OverdueItem
	setPreferenceCSV string
	setPreferenceErr error
	getPreference    *string
}

func (f *fakeUserDashboardService) GetMyBoard(ctx context.Context, email string) (user_dashboard_dto.Board, error) {
	return f.board, f.boardErr
}
func (f *fakeUserDashboardService) GetTeamView(ctx context.Context) (user_dashboard_dto.TeamView, error) {
	return f.teamView, f.teamViewErr
}
func (f *fakeUserDashboardService) GetOverdueView(ctx context.Context) ([]user_dashboard_dto.OverdueItem, error) {
	return f.overdueItems, nil
}
func (f *fakeUserDashboardService) SetReminderTierPreference(ctx context.Context, email, csv string) error {
	f.setPreferenceCSV = csv
	return f.setPreferenceErr
}
func (f *fakeUserDashboardService) GetReminderTierPreference(ctx context.Context, email string) (*string, error) {
	return f.getPreference, nil
}

type fakeChatInteractiveForHandler struct {
	lastSenderEmail string
	lastCommand     string
	response        string
}

func (f *fakeChatInteractiveForHandler) RouteCommand(ctx context.Context, senderEmail, messageText string) string {
	f.lastSenderEmail = senderEmail
	f.lastCommand = messageText
	return f.response
}

type fakeJiraCredentialForHandler struct {
	upsertErr     error
	upsertCalled  bool
	deleteCalled  bool
	maskedPreview string
	maskedFound   bool
}

func (f *fakeJiraCredentialForHandler) ResolveCredential(ctx context.Context, email string) (*jira_client_dto.Credential, error) {
	return nil, nil
}
func (f *fakeJiraCredentialForHandler) UpsertCredential(ctx context.Context, userID int64, jiraEmail, apiToken string) error {
	f.upsertCalled = true
	return f.upsertErr
}
func (f *fakeJiraCredentialForHandler) DeleteCredential(ctx context.Context, userID int64) error {
	f.deleteCalled = true
	return nil
}
func (f *fakeJiraCredentialForHandler) MaskedPreview(ctx context.Context, userID int64) (string, bool, error) {
	return f.maskedPreview, f.maskedFound, nil
}

func newTestContextWithUser(method, path, body string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
		c.Request = httptest.NewRequest(method, path, reader)
		c.Request.Header.Set("Content-Type", "application/json")
	} else {
		c.Request = httptest.NewRequest(method, path, nil)
	}
	c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "dev@esb.co.id"})
	return w, c
}

func TestGetBoard_ReturnsBoardPreferenceAndCredentialAsJSON(t *testing.T) {
	svc := &fakeUserDashboardService{board: user_dashboard_dto.Board{Columns: []user_dashboard_dto.BoardColumn{
		{Status: "Code Process", Cards: []user_dashboard_dto.Card{
			{IssueKey: "OMS-1", Summary: "Test ticket", Status: "Code Process", HasBudgetDetail: true, StoryPoints: 3, ElapsedLabel: "1 jam", RemainingLabel: "2 jam"},
		}},
	}}}
	cred := &fakeJiraCredentialForHandler{maskedPreview: "••••ab3f", maskedFound: true}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, cred, testCfg())

	w, c := newTestContextWithUser("GET", "/api/dashboard/board", "")
	handler.GetBoard(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"issueKey":"OMS-1"`) {
		t.Errorf("body missing board ticket: %s", body)
	}
	if !strings.Contains(body, `"tiers":[`) {
		t.Errorf("body missing tierPreference.tiers: %s", body)
	}
	if !strings.Contains(body, `"hasCredential":true`) || !strings.Contains(body, `"maskedPreview":"••••ab3f"`) {
		t.Errorf("body missing credential info: %s", body)
	}
}

func TestGetBoard_DefaultsAllTiersWhenPreferenceNeverSet(t *testing.T) {
	svc := &fakeUserDashboardService{getPreference: nil}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, testCfg())

	w, c := newTestContextWithUser("GET", "/api/dashboard/board", "")
	handler.GetBoard(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	for _, code := range []string{"2h", "1h", "30m", "15m", "5m", "1m"} {
		if !strings.Contains(w.Body.String(), `"`+code+`"`) {
			t.Errorf("body missing default tier %q: %s", code, w.Body.String())
		}
	}
}

func TestGetBoard_ReturnsServerErrorWhenServiceFails(t *testing.T) {
	svc := &fakeUserDashboardService{boardErr: errors.New("jira down")}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, testCfg())

	w, c := newTestContextWithUser("GET", "/api/dashboard/board", "")
	handler.GetBoard(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestUpdatePreference_PersistsSelectedTiers(t *testing.T) {
	svc := &fakeUserDashboardService{}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, testCfg())

	w, c := newTestContextWithUser("POST", "/api/dashboard/preference", `{"tiers":["1h","15m"]}`)
	handler.UpdatePreference(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if svc.setPreferenceCSV != "1h,15m" {
		t.Errorf("setPreferenceCSV = %q, want %q", svc.setPreferenceCSV, "1h,15m")
	}
}

func TestUpdatePreference_EmptyArrayPersistsAsEmptyString(t *testing.T) {
	svc := &fakeUserDashboardService{}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, testCfg())

	w, c := newTestContextWithUser("POST", "/api/dashboard/preference", `{"tiers":[]}`)
	handler.UpdatePreference(c)

	if w.Code != http.StatusOK || svc.setPreferenceCSV != "" {
		t.Errorf("status=%d setPreferenceCSV=%q, want 200 and \"\" (spec AC2.1 — deliberately none)", w.Code, svc.setPreferenceCSV)
	}
}

func TestTransitionTicket_DelegatesToRouteCommand(t *testing.T) {
	chat := &fakeChatInteractiveForHandler{response: "✅ OMS-1 berhasil digeser ke Code Review"}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, chat, &fakeJiraCredentialForHandler{}, testCfg())

	w, c := newTestContextWithUser("POST", "/api/dashboard/tickets/OMS-1/transition", `{"target":"code-review"}`)
	c.Params = gin.Params{{Key: "issueKey", Value: "OMS-1"}}
	handler.TransitionTicket(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if chat.lastSenderEmail != "dev@esb.co.id" {
		t.Errorf("RouteCommand sender = %q, want dev@esb.co.id", chat.lastSenderEmail)
	}
	if chat.lastCommand != "/u-geser<OMS-1>:code-review" {
		t.Errorf("RouteCommand command = %q, want /u-geser<OMS-1>:code-review", chat.lastCommand)
	}
	if !strings.Contains(w.Body.String(), "berhasil digeser") {
		t.Errorf("body = %q, want it to echo RouteCommand's response (spec AC4.2)", w.Body.String())
	}
}

func TestSaveCredential_CallsUpsert(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, testCfg())

	w, c := newTestContextWithUser("POST", "/api/dashboard/credential", `{"jiraEmail":"dev@esb.co.id","apiToken":"my-token"}`)
	handler.SaveCredential(c)

	if w.Code != http.StatusOK || !cred.upsertCalled {
		t.Errorf("status=%d upsertCalled=%v, want 200 and true", w.Code, cred.upsertCalled)
	}
}

func TestSaveCredential_RejectsMissingFields(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, testCfg())

	w, c := newTestContextWithUser("POST", "/api/dashboard/credential", `{"jiraEmail":""}`)
	handler.SaveCredential(c)

	if w.Code != http.StatusBadRequest || cred.upsertCalled {
		t.Errorf("status=%d upsertCalled=%v, want 400 and false", w.Code, cred.upsertCalled)
	}
}

func TestSaveCredential_ReturnsServiceUnavailableWhenKeyNotConfigured(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{upsertErr: jira_credential_service.ErrEncryptionKeyNotConfigured}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, testCfg())

	w, c := newTestContextWithUser("POST", "/api/dashboard/credential", `{"jiraEmail":"dev@esb.co.id","apiToken":"tok"}`)
	handler.SaveCredential(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestDeleteCredential_CallsDelete(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, testCfg())

	w, c := newTestContextWithUser("DELETE", "/api/dashboard/credential", "")
	handler.DeleteCredential(c)

	if w.Code != http.StatusOK || !cred.deleteCalled {
		t.Errorf("status=%d deleteCalled=%v, want 200 and true", w.Code, cred.deleteCalled)
	}
}

func TestGetTeam_ReturnsGroupsAsJSON(t *testing.T) {
	svc := &fakeUserDashboardService{teamView: user_dashboard_dto.TeamView{Groups: []user_dashboard_dto.TeamGroup{
		{AssigneeEmail: "teammate@esb.co.id", Cards: []user_dashboard_dto.Card{{IssueKey: "OMS-1", Summary: "Test"}}},
	}}}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, testCfg())

	w, c := newTestContextWithUser("GET", "/api/dashboard/team", "")
	handler.GetTeam(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"assigneeEmail":"teammate@esb.co.id"`) {
		t.Errorf("body missing group: %s", w.Body.String())
	}
}

func TestGetTeam_ReturnsServerErrorWhenServiceFails(t *testing.T) {
	svc := &fakeUserDashboardService{}
	svc.teamViewErr = errors.New("db down")
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, testCfg())

	w, c := newTestContextWithUser("GET", "/api/dashboard/team", "")
	handler.GetTeam(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
