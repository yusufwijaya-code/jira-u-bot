package user_dashboard_handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_credential/jira_credential_service"
	"nudge/modules/user_dashboard/user_dashboard_dto"
	"nudge/modules/user_setting/user_setting_dto"
)

type fakeUserDashboardService struct {
	board            user_dashboard_dto.Board
	boardErr         error
	teamView         user_dashboard_dto.TeamView
	teamViewErr      error
	overdueItems     []user_dashboard_dto.OverdueItem
	overdueErr       error

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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{})

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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{})

	w, c := newTestContextWithUser("GET", "/api/dashboard/board", "")
	handler.GetBoard(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetTeamAndOverdue_UseSessionEmail(t *testing.T) {
	svc := &fakeUserDashboardService{}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{})

	_, c := newTestContextWithUser("GET", "/api/dashboard/team?email=victim@esb.co.id", "")
	handler.GetTeam(c)
	_, c = newTestContextWithUser("GET", "/api/dashboard/overdue?email=victim@esb.co.id", "")
	handler.GetOverdue(c)

	if svc.teamEmail != "dev@esb.co.id" || svc.overdueEmail != "dev@esb.co.id" {
		t.Errorf("team=%q overdue=%q, want both the session's dev@esb.co.id", svc.teamEmail, svc.overdueEmail)
	}
}

func TestTransitionTicket_DelegatesToRouteCommand(t *testing.T) {
	chat := &fakeChatInteractiveForHandler{response: "✅ OMS-1 berhasil digeser ke Code Review"}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, chat, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{})

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
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, &fakeUserSettingsForHandler{})

	w, c := newTestContextWithUser("POST", "/api/dashboard/credential", `{"jiraEmail":"dev@esb.co.id","apiToken":"my-token"}`)
	handler.SaveCredential(c)

	if w.Code != http.StatusOK || !cred.upsertCalled {
		t.Errorf("status=%d upsertCalled=%v, want 200 and true", w.Code, cred.upsertCalled)
	}
}

func TestSaveCredential_RejectsMissingFields(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, &fakeUserSettingsForHandler{})

	w, c := newTestContextWithUser("POST", "/api/dashboard/credential", `{"jiraEmail":""}`)
	handler.SaveCredential(c)

	if w.Code != http.StatusBadRequest || cred.upsertCalled {
		t.Errorf("status=%d upsertCalled=%v, want 400 and false", w.Code, cred.upsertCalled)
	}
}

func TestSaveCredential_ReturnsServiceUnavailableWhenKeyNotConfigured(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{upsertErr: jira_credential_service.ErrEncryptionKeyNotConfigured}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, &fakeUserSettingsForHandler{})

	w, c := newTestContextWithUser("POST", "/api/dashboard/credential", `{"jiraEmail":"dev@esb.co.id","apiToken":"tok"}`)
	handler.SaveCredential(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestDeleteCredential_CallsDelete(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, &fakeUserSettingsForHandler{})

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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{})

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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{})

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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{})

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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{})

	w, c := newTestContextWithUser("GET", "/api/dashboard/overdue", "")
	handler.GetOverdue(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- user settings (Tech Spec "Dynamic User Notification Settings") ---

type fakeUserSettingsForHandler struct {
	setting   user_setting_dto.UserSetting
	getErr    error
	saveErr   error
	savedFor  int64
	savedIn   *user_setting_dto.NotificationInput
	mutedFor  int64
	mutedTo   *time.Time
	unmutedID int64
}

func (f *fakeUserSettingsForHandler) GetForEmail(ctx context.Context, email string) user_setting_dto.UserSetting {
	return user_setting_dto.Default()
}

func (f *fakeUserSettingsForHandler) GetForUser(ctx context.Context, userID int64) (user_setting_dto.UserSetting, error) {
	if f.getErr != nil {
		return user_setting_dto.UserSetting{}, f.getErr
	}
	if f.setting.ReminderTiers == nil {
		return user_setting_dto.Default(), nil
	}
	return f.setting, nil
}

func (f *fakeUserSettingsForHandler) SaveNotification(ctx context.Context, userID int64, in user_setting_dto.NotificationInput) (user_setting_dto.UserSetting, error) {
	f.savedFor, f.savedIn = userID, &in
	if f.saveErr != nil {
		return user_setting_dto.UserSetting{}, f.saveErr
	}
	return in.Apply(user_setting_dto.Default())
}

func (f *fakeUserSettingsForHandler) MuteUntil(ctx context.Context, userID int64, until time.Time) error {
	f.mutedFor, f.mutedTo = userID, &until
	return nil
}

func (f *fakeUserSettingsForHandler) Unmute(ctx context.Context, userID int64) error {
	f.unmutedID = userID
	return nil
}

func settingsHandler(settings *fakeUserSettingsForHandler) UserDashboardHandler {
	cred := &fakeJiraCredentialForHandler{maskedPreview: "••••ab3f", maskedFound: true}
	return NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, settings)
}

func TestGetSettings_ReturnsNotificationMuteAndCredential(t *testing.T) {
	setting := user_setting_dto.Default()
	setting.ReminderTiers = []string{"2h", "45m"}
	setting.OverageTiers = []string{"15m"}
	until := time.Now().Add(time.Hour)
	setting.MutedUntil = &until
	setting.QuietHours = &user_setting_dto.QuietHours{Start: "12:00", End: "13:00"}
	handler := settingsHandler(&fakeUserSettingsForHandler{setting: setting})

	w, c := newTestContextWithUser("GET", "/api/dashboard/settings", "")
	handler.GetSettings(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`"reminderTiers":["2h","45m"]`,
		`"overageTiers":["15m"]`,
		`"overageRepeatMinutes":5`,
		`"quietHours":{"start":"12:00","end":"13:00"}`,
		`"active":true`,
		`"hasCredential":true`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
}

func TestGetSettings_DefaultsForUserWithoutSettings(t *testing.T) {
	handler := settingsHandler(&fakeUserSettingsForHandler{})

	w, c := newTestContextWithUser("GET", "/api/dashboard/settings", "")
	handler.GetSettings(c)

	body := w.Body.String()
	if !strings.Contains(body, `"reminderTiers":["2h","1h","30m","15m","5m","1m"]`) || !strings.Contains(body, `"overageTiers":[]`) || !strings.Contains(body, `"quietHours":null`) {
		t.Errorf("body = %s, want defaults", body)
	}
	if !strings.Contains(body, `"mute":{"active":false,"forever":false,"until":null}`) {
		t.Errorf("body = %s, want mute inactive", body)
	}
}

func TestGetSettings_ReturnsServerErrorWhenLoadFails(t *testing.T) {
	handler := settingsHandler(&fakeUserSettingsForHandler{getErr: errors.New("db down")})

	w, c := newTestContextWithUser("GET", "/api/dashboard/settings", "")
	handler.GetSettings(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestUpdateNotificationSettings_SavesForSessionUser(t *testing.T) {
	settings := &fakeUserSettingsForHandler{}
	handler := settingsHandler(settings)

	w, c := newTestContextWithUser("PUT", "/api/dashboard/settings/notification",
		`{"reminderTiers":["60m","15m"],"overageTiers":["30m"],"overageRepeatMinutes":null,"notifyEarlyFinish":false,"notifyReturnedToTodo":true,"quietHours":null}`)
	handler.UpdateNotificationSettings(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if settings.savedIn == nil || settings.savedFor == 0 {
		t.Fatal("SaveNotification was not called for the session user")
	}
	if !strings.Contains(w.Body.String(), `"reminderTiers":["1h","15m"]`) || !strings.Contains(w.Body.String(), `"overageRepeatMinutes":null`) {
		t.Errorf("body = %s, want the normalized saved settings", w.Body.String())
	}
}

func TestUpdateNotificationSettings_ValidationErrorIs400WithField(t *testing.T) {
	handler := settingsHandler(&fakeUserSettingsForHandler{})

	w, c := newTestContextWithUser("PUT", "/api/dashboard/settings/notification", `{"reminderTiers":["9h"]}`)
	handler.UpdateNotificationSettings(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"fields":{"reminderTiers"`) {
		t.Errorf("body = %s, want a reminderTiers field error", w.Body.String())
	}
}

func TestMute_ForMinutes(t *testing.T) {
	settings := &fakeUserSettingsForHandler{}
	handler := settingsHandler(settings)

	before := time.Now()
	w, c := newTestContextWithUser("POST", "/api/dashboard/settings/mute", `{"minutes":60}`)
	handler.Mute(c)

	if w.Code != http.StatusOK || settings.mutedTo == nil {
		t.Fatalf("status = %d, mutedTo = %v", w.Code, settings.mutedTo)
	}
	if d := settings.mutedTo.Sub(before); d < 59*time.Minute || d > 61*time.Minute {
		t.Errorf("muted for %v, want ~60m", d)
	}
	if !strings.Contains(w.Body.String(), `"active":true`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestMute_UntilTomorrow(t *testing.T) {
	settings := &fakeUserSettingsForHandler{}
	handler := settingsHandler(settings)

	w, c := newTestContextWithUser("POST", "/api/dashboard/settings/mute", `{"until":"tomorrow"}`)
	handler.Mute(c)

	if w.Code != http.StatusOK || settings.mutedTo == nil {
		t.Fatalf("status = %d, mutedTo = %v", w.Code, settings.mutedTo)
	}
	local := settings.mutedTo.In(user_setting_dto.Jakarta)
	if local.Hour() != 8 || local.Minute() != 30 {
		t.Errorf("muted until %v, want 08:30 WIB", local)
	}
}

func TestMute_Forever(t *testing.T) {
	settings := &fakeUserSettingsForHandler{}
	handler := settingsHandler(settings)

	w, c := newTestContextWithUser("POST", "/api/dashboard/settings/mute", `{"until":"forever"}`)
	handler.Mute(c)

	if w.Code != http.StatusOK || settings.mutedTo == nil || settings.mutedTo.Year() != 9999 {
		t.Fatalf("status = %d, mutedTo = %v; want the 9999 forever sentinel", w.Code, settings.mutedTo)
	}
	if !strings.Contains(w.Body.String(), `"forever":true`) {
		t.Errorf("body = %s, want forever:true", w.Body.String())
	}
}

func TestGetSettings_ReportsForeverMute(t *testing.T) {
	setting := user_setting_dto.Default()
	forever := user_setting_dto.MuteForeverUntil
	setting.MutedUntil = &forever
	handler := settingsHandler(&fakeUserSettingsForHandler{setting: setting})

	w, c := newTestContextWithUser("GET", "/api/dashboard/settings", "")
	handler.GetSettings(c)

	if !strings.Contains(w.Body.String(), `"active":true,"forever":true`) {
		t.Errorf("body = %s, want an active forever mute", w.Body.String())
	}
}

func TestMute_RejectsInvalidRequests(t *testing.T) {
	for _, body := range []string{`{}`, `{"minutes":0}`, `{"minutes":2000}`, `{"until":"nextweek"}`, `{"minutes":10,"until":"tomorrow"}`, `{"minutes":10,"until":"forever"}`} {
		settings := &fakeUserSettingsForHandler{}
		handler := settingsHandler(settings)
		w, c := newTestContextWithUser("POST", "/api/dashboard/settings/mute", body)
		handler.Mute(c)
		if w.Code != http.StatusBadRequest || settings.mutedTo != nil {
			t.Errorf("body %s: status = %d, muted = %v; want 400 and no mute", body, w.Code, settings.mutedTo)
		}
	}
}

func TestUnmute(t *testing.T) {
	settings := &fakeUserSettingsForHandler{}
	handler := settingsHandler(settings)

	w, c := newTestContextWithUser("DELETE", "/api/dashboard/settings/mute", "")
	handler.Unmute(c)

	if w.Code != http.StatusOK || settings.unmutedID == 0 {
		t.Errorf("status = %d, unmutedID = %d", w.Code, settings.unmutedID)
	}
	if !strings.Contains(w.Body.String(), `"active":false`) {
		t.Errorf("body = %s", w.Body.String())
	}
}
