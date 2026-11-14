package user_dashboard_handler

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
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_credential/jira_credential_service"
	"nudge/modules/user_dashboard/user_dashboard_dto"
)

type fakeUserDashboardService struct {
	board            user_dashboard_dto.Board
	boardErr         error
	teamView         user_dashboard_dto.TeamView
	teamViewErr      error
	overdueItems     []user_dashboard_dto.OverdueItem
	overdueErr       error
	setPreferenceCSV string
	setPreferenceErr error
	getPreference    *string
	getPreferenceErr error

	// emails the handler passed to each read method, to prove the identity
	// always comes from the session.
	boardEmail   string
	teamEmail    string
	overdueEmail string
}

func (f *fakeUserDashboardService) GetMyBoard(ctx context.Context, email string) (user_dashboard_dto.Board, error) {
	f.boardEmail = email
	return f.board, f.boardErr
}
func (f *fakeUserDashboardService) GetTeamView(ctx context.Context, email string) (user_dashboard_dto.TeamView, error) {
	f.teamEmail = email
	return f.teamView, f.teamViewErr
}
func (f *fakeUserDashboardService) GetOverdueView(ctx context.Context, email string) ([]user_dashboard_dto.OverdueItem, error) {
	f.overdueEmail = email
	return f.overdueItems, f.overdueErr
}
func (f *fakeUserDashboardService) SetReminderTierPreference(ctx context.Context, email, csv string) error {
	f.setPreferenceCSV = csv
	return f.setPreferenceErr
}
func (f *fakeUserDashboardService) GetReminderTierPreference(ctx context.Context, email string) (*string, error) {
	return f.getPreference, f.getPreferenceErr
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

func TestGetBoard_ReturnsBoardAsJSONForSessionEmail(t *testing.T) {
	svc := &fakeUserDashboardService{board: user_dashboard_dto.Board{Columns: []user_dashboard_dto.BoardColumn{
		{Status: "Code Process", Cards: []user_dashboard_dto.Card{
			{IssueKey: "OMS-1", Summary: "Test ticket", Status: "Code Process", HasBudgetDetail: true, StoryPoints: 3, ElapsedLabel: "1 jam", RemainingLabel: "2 jam"},
		}},
	}}}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{})

	// A hostile ?email= must be ignored - identity only comes from the session.
	w, c := newTestContextWithUser("GET", "/api/dashboard/board?email=victim@esb.co.id", "")
	handler.GetBoard(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"issueKey":"OMS-1"`) {
		t.Errorf("body missing board ticket: %s", w.Body.String())
	}
	if svc.boardEmail != "dev@esb.co.id" {
		t.Errorf("GetMyBoard called with %q, want the session's dev@esb.co.id", svc.boardEmail)
	}
}

func TestGetBoard_ReturnsServerErrorWhenServiceFails(t *testing.T) {
	svc := &fakeUserDashboardService{boardErr: errors.New("jira down")}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{})

	w, c := newTestContextWithUser("GET", "/api/dashboard/board", "")
	handler.GetBoard(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetSettings_ReturnsPreferenceAndCredentialAsJSON(t *testing.T) {
	saved := "2h,1h"
	svc := &fakeUserDashboardService{getPreference: &saved}
	cred := &fakeJiraCredentialForHandler{maskedPreview: "••••ab3f", maskedFound: true}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, cred)

	w, c := newTestContextWithUser("GET", "/api/dashboard/settings", "")
	handler.GetSettings(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"tiers":["2h","1h"]`) {
		t.Errorf("body missing saved tierPreference.tiers: %s", body)
	}
	if !strings.Contains(body, `"hasCredential":true`) || !strings.Contains(body, `"maskedPreview":"••••ab3f"`) {
		t.Errorf("body missing credential info: %s", body)
	}
}

func TestGetSettings_DefaultsAllTiersWhenPreferenceNeverSet(t *testing.T) {
	svc := &fakeUserDashboardService{getPreference: nil}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{})

	w, c := newTestContextWithUser("GET", "/api/dashboard/settings", "")
	handler.GetSettings(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	for _, code := range []string{"2h", "1h", "30m", "15m", "5m", "1m"} {
		if !strings.Contains(w.Body.String(), `"`+code+`"`) {
			t.Errorf("body missing default tier %q: %s", code, w.Body.String())
		}
	}
}

func TestGetSettings_ReturnsServerErrorWhenServiceFails(t *testing.T) {
	svc := &fakeUserDashboardService{getPreferenceErr: errors.New("db down")}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{})

	w, c := newTestContextWithUser("GET", "/api/dashboard/settings", "")
	handler.GetSettings(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetTeamAndOverdue_UseSessionEmail(t *testing.T) {
	svc := &fakeUserDashboardService{}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{})

	_, c := newTestContextWithUser("GET", "/api/dashboard/team?email=victim@esb.co.id", "")
	handler.GetTeam(c)
	_, c = newTestContextWithUser("GET", "/api/dashboard/overdue?email=victim@esb.co.id", "")
	handler.GetOverdue(c)

	if svc.teamEmail != "dev@esb.co.id" || svc.overdueEmail != "dev@esb.co.id" {
		t.Errorf("team=%q overdue=%q, want both the session's dev@esb.co.id", svc.teamEmail, svc.overdueEmail)
	}
}

func TestUpdatePreference_PersistsSelectedTiers(t *testing.T) {
	svc := &fakeUserDashboardService{}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{})

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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{})

	w, c := newTestContextWithUser("POST", "/api/dashboard/preference", `{"tiers":[]}`)
	handler.UpdatePreference(c)

	if w.Code != http.StatusOK || svc.setPreferenceCSV != "" {
		t.Errorf("status=%d setPreferenceCSV=%q, want 200 and \"\" (spec AC2.1 — deliberately none)", w.Code, svc.setPreferenceCSV)
	}
}

func TestTransitionTicket_DelegatesToRouteCommand(t *testing.T) {
	chat := &fakeChatInteractiveForHandler{response: "✅ OMS-1 berhasil digeser ke Code Review"}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, chat, &fakeJiraCredentialForHandler{})

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
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred)

	w, c := newTestContextWithUser("POST", "/api/dashboard/credential", `{"jiraEmail":"dev@esb.co.id","apiToken":"my-token"}`)
	handler.SaveCredential(c)

	if w.Code != http.StatusOK || !cred.upsertCalled {
		t.Errorf("status=%d upsertCalled=%v, want 200 and true", w.Code, cred.upsertCalled)
	}
}

func TestSaveCredential_RejectsMissingFields(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred)

	w, c := newTestContextWithUser("POST", "/api/dashboard/credential", `{"jiraEmail":""}`)
	handler.SaveCredential(c)

	if w.Code != http.StatusBadRequest || cred.upsertCalled {
		t.Errorf("status=%d upsertCalled=%v, want 400 and false", w.Code, cred.upsertCalled)
	}
}

func TestSaveCredential_ReturnsServiceUnavailableWhenKeyNotConfigured(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{upsertErr: jira_credential_service.ErrEncryptionKeyNotConfigured}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred)

	w, c := newTestContextWithUser("POST", "/api/dashboard/credential", `{"jiraEmail":"dev@esb.co.id","apiToken":"tok"}`)
	handler.SaveCredential(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestDeleteCredential_CallsDelete(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred)

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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{})

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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{})

	w, c := newTestContextWithUser("GET", "/api/dashboard/team", "")
	handler.GetTeam(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetOverdue_ReturnsItemsAsJSON(t *testing.T) {
	svc := &fakeUserDashboardService{overdueItems: []user_dashboard_dto.OverdueItem{
		{Card: user_dashboard_dto.Card{IssueKey: "OMS-1", Summary: "Test"}, AssigneeEmail: "teammate@esb.co.id"},
	}}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{})

	w, c := newTestContextWithUser("GET", "/api/dashboard/overdue", "")
	handler.GetOverdue(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"issueKey":"OMS-1"`) || !strings.Contains(w.Body.String(), `"assigneeEmail":"teammate@esb.co.id"`) {
		t.Errorf("body missing overdue item: %s", w.Body.String())
	}
}

func TestGetOverdue_ReturnsServerErrorWhenServiceFails(t *testing.T) {
	svc := &fakeUserDashboardService{}
	svc.overdueErr = errors.New("db down")
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{})

	w, c := newTestContextWithUser("GET", "/api/dashboard/overdue", "")
	handler.GetOverdue(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
