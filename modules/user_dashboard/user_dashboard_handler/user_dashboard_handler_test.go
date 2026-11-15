package user_dashboard_handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"nudge/modules/board/board_service"
	"nudge/modules/user_setting/user_setting_service"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_credential/jira_credential_service"
	"nudge/modules/user_dashboard/user_dashboard_dto"
	"nudge/modules/user_dashboard/user_dashboard_service"
	"nudge/modules/user_setting/user_setting_dto"
)

type fakeUserDashboardService struct {
	board        user_dashboard_dto.Board
	boardErr     error
	teamView     user_dashboard_dto.TeamView
	teamViewErr  error
	overdueItems []user_dashboard_dto.OverdueItem
	overdueErr   error

	// emails the handler passed to each read method, to prove the identity
	// always comes from the session.
	boardEmail   string
	boardID      int64
	teamEmail    string
	overdueEmail string

	boards      []user_dashboard_dto.BoardSummary
	boardsEmail string
	// boardNames: boardID -> {name, projectKey} for DescribeBoard.
	boardNames map[int64][2]string

	// Tech Spec "Papan Tiket & Cuti": ListTransitions / Transition.
	transitions        user_dashboard_dto.TicketTransitions
	transitionsErr     error
	transitionStatus   string
	transitionErr      error
	transitionEmail    string
	transitionKey      string
	transitionIDCalled string
}

func (f *fakeUserDashboardService) ListTransitions(ctx context.Context, email, issueKey string) (user_dashboard_dto.TicketTransitions, error) {
	f.transitionEmail, f.transitionKey = email, issueKey
	return f.transitions, f.transitionsErr
}

func (f *fakeUserDashboardService) Transition(ctx context.Context, email, issueKey, transitionID string) (string, error) {
	f.transitionEmail, f.transitionKey, f.transitionIDCalled = email, issueKey, transitionID
	return f.transitionStatus, f.transitionErr
}

func (f *fakeUserDashboardService) GetMyBoard(ctx context.Context, email string, boardID int64) (user_dashboard_dto.Board, error) {
	f.boardEmail, f.boardID = email, boardID
	return f.board, f.boardErr
}

func (f *fakeUserDashboardService) ListBoards(ctx context.Context, email string) ([]user_dashboard_dto.BoardSummary, error) {
	f.boardsEmail = email
	return f.boards, nil
}
func (f *fakeUserDashboardService) DescribeBoard(ctx context.Context, boardID int64) (string, string) {
	if n, ok := f.boardNames[boardID]; ok {
		return n[0], n[1]
	}
	return fmt.Sprintf("Board #%d", boardID), ""
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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, nil)

	// A hostile ?email= must be ignored - identity only comes from the session.
	w, c := newTestContextWithUser("GET", "/api/dashboard/board?email=victim@esb.co.id", "")
	handler.GetBoard(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"issueKey":"OMS-1"`) {
		t.Errorf("body missing board ticket: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"related":[]`) {
		t.Errorf("body must carry related as [] (not null): %s", w.Body.String())
	}
	if svc.boardEmail != "dev@esb.co.id" {
		t.Errorf("GetMyBoard called with %q, want the session's dev@esb.co.id", svc.boardEmail)
	}
}

func TestGetBoard_ReturnsServerErrorWhenServiceFails(t *testing.T) {
	svc := &fakeUserDashboardService{boardErr: errors.New("jira down")}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, nil)

	w, c := newTestContextWithUser("GET", "/api/dashboard/board", "")
	handler.GetBoard(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetTeamAndOverdue_UseSessionEmail(t *testing.T) {
	svc := &fakeUserDashboardService{}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, nil)

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
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, chat, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, nil)

	w, c := newTestContextWithUser("POST", "/api/dashboard/tickets/OMS-1/transition", `{"target":"code-review"}`)
	c.Params = gin.Params{{Key: "issueKey", Value: "OMS-1"}}
	handler.TransitionTicket(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if chat.lastSenderEmail != "dev@esb.co.id" {
		t.Errorf("RouteCommand sender = %q, want dev@esb.co.id", chat.lastSenderEmail)
	}
	if chat.lastCommand != "/n-geser<OMS-1>:code-review" {
		t.Errorf("RouteCommand command = %q, want /n-geser<OMS-1>:code-review", chat.lastCommand)
	}
	if !strings.Contains(w.Body.String(), "berhasil digeser") {
		t.Errorf("body = %q, want it to echo RouteCommand's response (spec AC4.2)", w.Body.String())
	}
}

func TestSaveCredential_CallsUpsert(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, &fakeUserSettingsForHandler{}, nil)

	w, c := newTestContextWithUser("POST", "/api/dashboard/credential", `{"jiraEmail":"dev@esb.co.id","apiToken":"my-token"}`)
	handler.SaveCredential(c)

	if w.Code != http.StatusOK || !cred.upsertCalled {
		t.Errorf("status=%d upsertCalled=%v, want 200 and true", w.Code, cred.upsertCalled)
	}
}

func TestSaveCredential_RejectsMissingFields(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, &fakeUserSettingsForHandler{}, nil)

	w, c := newTestContextWithUser("POST", "/api/dashboard/credential", `{"jiraEmail":""}`)
	handler.SaveCredential(c)

	if w.Code != http.StatusBadRequest || cred.upsertCalled {
		t.Errorf("status=%d upsertCalled=%v, want 400 and false", w.Code, cred.upsertCalled)
	}
}

func TestSaveCredential_ReturnsServiceUnavailableWhenKeyNotConfigured(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{upsertErr: jira_credential_service.ErrEncryptionKeyNotConfigured}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, &fakeUserSettingsForHandler{}, nil)

	w, c := newTestContextWithUser("POST", "/api/dashboard/credential", `{"jiraEmail":"dev@esb.co.id","apiToken":"tok"}`)
	handler.SaveCredential(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestDeleteCredential_CallsDelete(t *testing.T) {
	cred := &fakeJiraCredentialForHandler{}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, &fakeUserSettingsForHandler{}, nil)

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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, nil)

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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, nil)

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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, nil)

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
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, nil)

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

	// board each call was scoped to
	gotBoard, savedBoard, mutedBoard, unmutedBoard, resetBoard int64
	states                                                     map[int64]user_setting_service.BoardState

	// master switch (Tech Spec "Notification On-Off Consent")
	notifSwitch    user_setting_dto.NotificationSwitch
	switchErr      error
	switchSavedFor int64
	switchSavedTo  *bool
}

func (f *fakeUserSettingsForHandler) NotificationSwitch(ctx context.Context, userID int64) (user_setting_dto.NotificationSwitch, error) {
	return f.notifSwitch, f.switchErr
}

func (f *fakeUserSettingsForHandler) SetNotificationsEnabled(ctx context.Context, userID int64, enabled bool) (user_setting_dto.NotificationSwitch, error) {
	if f.switchErr != nil {
		return user_setting_dto.NotificationSwitch{}, f.switchErr
	}
	f.switchSavedFor, f.switchSavedTo = userID, &enabled
	f.notifSwitch = user_setting_dto.NotificationSwitch{Enabled: enabled, Chosen: true}
	return f.notifSwitch, nil
}

func (f *fakeUserSettingsForHandler) GetEffectiveForEmail(ctx context.Context, email string, boardID int64) user_setting_dto.UserSetting {
	return user_setting_dto.Default()
}

func (f *fakeUserSettingsForHandler) GetEffective(ctx context.Context, userID, boardID int64) (user_setting_dto.UserSetting, error) {
	f.gotBoard = boardID
	if f.getErr != nil {
		return user_setting_dto.UserSetting{}, f.getErr
	}
	if f.setting.ReminderTiers == nil {
		d := user_setting_dto.Default()
		d.BoardID, d.UseDefault = boardID, boardID != 0
		return d, nil
	}
	return f.setting, nil
}

func (f *fakeUserSettingsForHandler) SaveNotification(ctx context.Context, userID, boardID int64, in user_setting_dto.NotificationInput) (user_setting_dto.UserSetting, error) {
	f.savedFor, f.savedIn, f.savedBoard = userID, &in, boardID
	if f.saveErr != nil {
		return user_setting_dto.UserSetting{}, f.saveErr
	}
	return in.Apply(user_setting_dto.Default())
}

func (f *fakeUserSettingsForHandler) ResetToDefault(ctx context.Context, userID, boardID int64) error {
	f.resetBoard = boardID
	return nil
}

func (f *fakeUserSettingsForHandler) MuteUntil(ctx context.Context, userID, boardID int64, until time.Time) error {
	f.mutedFor, f.mutedTo, f.mutedBoard = userID, &until, boardID
	return nil
}

func (f *fakeUserSettingsForHandler) Unmute(ctx context.Context, userID, boardID int64) error {
	f.unmutedID, f.unmutedBoard = userID, boardID
	return nil
}

func (f *fakeUserSettingsForHandler) BoardStates(ctx context.Context, userID int64) (map[int64]user_setting_service.BoardState, error) {
	return f.states, nil
}

func settingsHandler(settings *fakeUserSettingsForHandler) UserDashboardHandler {
	cred := &fakeJiraCredentialForHandler{maskedPreview: "••••ab3f", maskedFound: true}
	return NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, cred, settings, nil)
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
	if !strings.Contains(body, `"global":{"active":false,"forever":false,"until":null}`) {
		t.Errorf("body = %s, want mute inactive", body)
	}
}

func TestGetSettings_ReturnsTheNotificationSwitch(t *testing.T) {
	handler := settingsHandler(&fakeUserSettingsForHandler{notifSwitch: user_setting_dto.NotificationSwitch{Enabled: false, Chosen: false}})
	w, c := newTestContextWithUser("GET", "/api/dashboard/settings", "")
	handler.GetSettings(c)
	if !strings.Contains(w.Body.String(), `"notifications":{"chosen":false,"enabled":false}`) {
		t.Errorf("body = %s, want the not-yet-chosen switch", w.Body.String())
	}

	// A failed switch read must not trap the user in the consent pop up.
	handler = settingsHandler(&fakeUserSettingsForHandler{switchErr: errors.New("db down")})
	w, c = newTestContextWithUser("GET", "/api/dashboard/settings", "")
	handler.GetSettings(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"notifications":{"chosen":true,"enabled":true}`) {
		t.Errorf("status=%d body=%s, want 200 with chosen+enabled fallback", w.Code, w.Body.String())
	}
}

func TestUpdateNotificationsSwitch_SavesForSessionUser(t *testing.T) {
	settings := &fakeUserSettingsForHandler{}
	handler := settingsHandler(settings)

	w, c := newTestContextWithUser("PUT", "/api/dashboard/settings/notifications", `{"enabled":true}`)
	handler.UpdateNotificationsSwitch(c)

	if w.Code != http.StatusOK || w.Body.String() != `{"chosen":true,"enabled":true}` {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if settings.switchSavedFor != 1 || settings.switchSavedTo == nil || !*settings.switchSavedTo {
		t.Errorf("saved for user %d to %v, want user 1 (session) on", settings.switchSavedFor, settings.switchSavedTo)
	}
}

func TestUpdateNotificationsSwitch_RejectsMissingValueAndReportsFailure(t *testing.T) {
	for _, body := range []string{`{}`, `{"enabled":"yes"}`, `not json`} {
		settings := &fakeUserSettingsForHandler{}
		w, c := newTestContextWithUser("PUT", "/api/dashboard/settings/notifications", body)
		settingsHandler(settings).UpdateNotificationsSwitch(c)
		if w.Code != http.StatusBadRequest || settings.switchSavedTo != nil {
			t.Errorf("body %q: status=%d saved=%v, want 400 and nothing saved", body, w.Code, settings.switchSavedTo)
		}
	}

	w, c := newTestContextWithUser("PUT", "/api/dashboard/settings/notifications", `{"enabled":false}`)
	settingsHandler(&fakeUserSettingsForHandler{switchErr: errors.New("db down")}).UpdateNotificationsSwitch(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
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

	if !strings.Contains(w.Body.String(), `"global":{"active":true,"forever":true`) {
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

// --- per board (Tech Spec "Per-Board Settings & Active Sprint Dashboard") ---

type fakeAvatars struct {
	body  []byte
	err   error
	asked int64
}

func (f *fakeAvatars) Avatar(ctx context.Context, boardID int64) ([]byte, string, error) {
	f.asked = boardID
	return f.body, "image/svg+xml", f.err
}

func TestBoardParam_RejectsNonNumbers(t *testing.T) {
	handler := settingsHandler(&fakeUserSettingsForHandler{})
	for _, path := range []string{"/api/dashboard/settings?board=abc", "/api/dashboard/settings?board=-1"} {
		w, c := newTestContextWithUser("GET", path, "")
		handler.GetSettings(c)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", path, w.Code)
		}
	}
}

func TestGetSettings_BoardScopeReportsUseDefaultAndBothMutes(t *testing.T) {
	setting := user_setting_dto.Default()
	setting.BoardID, setting.UseDefault = 28, true
	global := time.Now().Add(time.Hour)
	board := user_setting_dto.MuteForeverUntil
	setting.MutedUntil, setting.BoardMutedUntil = &global, &board
	settings := &fakeUserSettingsForHandler{setting: setting}
	handler := settingsHandler(settings)

	w, c := newTestContextWithUser("GET", "/api/dashboard/settings?board=28", "")
	handler.GetSettings(c)

	body := w.Body.String()
	if settings.gotBoard != 28 {
		t.Errorf("GetEffective board = %d, want 28", settings.gotBoard)
	}
	for _, want := range []string{`"scope":"board"`, `"boardId":28`, `"useDefault":true`, `"board":{"active":true,"forever":true`, `"global":{"active":true,"forever":false`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
}

func TestGetSettings_DefaultScopeHasNoBoardMute(t *testing.T) {
	handler := settingsHandler(&fakeUserSettingsForHandler{})
	w, c := newTestContextWithUser("GET", "/api/dashboard/settings", "")
	handler.GetSettings(c)
	if body := w.Body.String(); !strings.Contains(body, `"scope":"default"`) || !strings.Contains(body, `"board":null`) {
		t.Errorf("body = %s", body)
	}
}

func TestSettingsWritesAreScopedToTheBoardParam(t *testing.T) {
	settings := &fakeUserSettingsForHandler{}
	handler := settingsHandler(settings)

	w, c := newTestContextWithUser("PUT", "/api/dashboard/settings/notification?board=26", `{"reminderTiers":["1h"]}`)
	handler.UpdateNotificationSettings(c)
	if w.Code != http.StatusOK || settings.savedBoard != 26 {
		t.Errorf("save: status %d, board %d", w.Code, settings.savedBoard)
	}

	w, c = newTestContextWithUser("POST", "/api/dashboard/settings/mute?board=26", `{"minutes":60}`)
	handler.Mute(c)
	if w.Code != http.StatusOK || settings.mutedBoard != 26 {
		t.Errorf("mute: status %d, board %d", w.Code, settings.mutedBoard)
	}

	w, c = newTestContextWithUser("DELETE", "/api/dashboard/settings/mute?board=26", "")
	handler.Unmute(c)
	if w.Code != http.StatusOK || settings.unmutedBoard != 26 {
		t.Errorf("unmute: status %d, board %d", w.Code, settings.unmutedBoard)
	}

	w, c = newTestContextWithUser("POST", "/api/dashboard/settings/mute", `{"minutes":60}`)
	handler.Mute(c)
	if settings.mutedBoard != 0 {
		t.Errorf("mute without board = board %d, want 0 (global)", settings.mutedBoard)
	}
}

func TestResetNotificationSettings(t *testing.T) {
	settings := &fakeUserSettingsForHandler{}
	handler := settingsHandler(settings)

	w, c := newTestContextWithUser("DELETE", "/api/dashboard/settings/notification", "")
	handler.ResetNotificationSettings(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("default scope: status = %d, want 400", w.Code)
	}

	w, c = newTestContextWithUser("DELETE", "/api/dashboard/settings/notification?board=28", "")
	handler.ResetNotificationSettings(c)
	if w.Code != http.StatusOK || settings.resetBoard != 28 {
		t.Errorf("board: status %d, reset %d", w.Code, settings.resetBoard)
	}
}

func TestGetBoard_PassesTheBoardFilterForTheSessionUser(t *testing.T) {
	svc := &fakeUserDashboardService{}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, nil)

	w, c := newTestContextWithUser("GET", "/api/dashboard/board?board=28&email=victim@esb.co.id", "")
	handler.GetBoard(c)

	if w.Code != http.StatusOK || svc.boardID != 28 || svc.boardEmail != "dev@esb.co.id" {
		t.Errorf("status %d, board %d, email %q", w.Code, svc.boardID, svc.boardEmail)
	}
}

func TestListBoards_MergesCustomAndMuteState(t *testing.T) {
	svc := &fakeUserDashboardService{boards: []user_dashboard_dto.BoardSummary{
		{BoardID: 26, Name: "ESB Core Board", ProjectKey: "CORE", TicketCount: 1, ReviewTracked: true},
		{BoardID: 28, Name: "ESB OMS Board", ProjectKey: "OMS", TicketCount: 17, ActiveSprint: user_dashboard_dto.SprintSummary{ID: 8294, Name: "OMS Sprint 23 - 2026"}},
	}}
	until := time.Now().Add(time.Hour)
	settings := &fakeUserSettingsForHandler{states: map[int64]user_setting_service.BoardState{
		28: {BoardID: 28, Custom: true, MutedUntil: &until},
	}}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, settings, nil)

	w, c := newTestContextWithUser("GET", "/api/dashboard/boards", "")
	handler.ListBoards(c)

	body := w.Body.String()
	if w.Code != http.StatusOK || svc.boardsEmail != "dev@esb.co.id" {
		t.Fatalf("status %d, email %q", w.Code, svc.boardsEmail)
	}
	for _, want := range []string{`"boardId":28`, `"ticketCount":17`, `"hasCustomSettings":true`, `"name":"OMS Sprint 23 - 2026"`, `"muted":{"active":true`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
	if !strings.Contains(body, `"reviewTracked":true`) || !strings.Contains(body, `"reviewTracked":false`) {
		t.Errorf("body must report reviewTracked per board: %s", body)
	}
	if !strings.Contains(body, `"hasCustomSettings":false`) {
		t.Errorf("board 26 must report hasCustomSettings false: %s", body)
	}
}

func TestBoardAvatar(t *testing.T) {
	avatars := &fakeAvatars{body: []byte("<svg/>")}
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, avatars)

	w, c := newTestContextWithUser("GET", "/api/dashboard/boards/28/avatar", "")
	c.Params = gin.Params{{Key: "boardId", Value: "28"}}
	handler.BoardAvatar(c)
	if w.Code != http.StatusOK || w.Body.String() != "<svg/>" || w.Header().Get("Content-Type") != "image/svg+xml" || avatars.asked != 28 {
		t.Errorf("ok case: status %d, body %q, ct %q", w.Code, w.Body.String(), w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Header().Get("Cache-Control"), "max-age=86400") {
		t.Errorf("Cache-Control = %q", w.Header().Get("Cache-Control"))
	}

	avatars.err = board_service.ErrAvatarNotFound
	w, c = newTestContextWithUser("GET", "/api/dashboard/boards/999/avatar", "")
	c.Params = gin.Params{{Key: "boardId", Value: "999"}}
	handler.BoardAvatar(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown board: status = %d, want 404", w.Code)
	}

	avatars.err = errors.New("jira down")
	w, c = newTestContextWithUser("GET", "/api/dashboard/boards/28/avatar", "")
	c.Params = gin.Params{{Key: "boardId", Value: "28"}}
	handler.BoardAvatar(c)
	if w.Code != http.StatusBadGateway {
		t.Errorf("jira error: status = %d, want 502", w.Code)
	}

	w, c = newTestContextWithUser("GET", "/api/dashboard/boards/x/avatar", "")
	c.Params = gin.Params{{Key: "boardId", Value: "x"}}
	handler.BoardAvatar(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad id: status = %d, want 400", w.Code)
	}
}

// --- Tech Spec "Papan Tiket & Cuti" (US-1) ---

func TestTransitionTicket_WithTransitionIDUsesWorkflowService(t *testing.T) {
	svc := &fakeUserDashboardService{transitionStatus: "Code Process"}
	chat := &fakeChatInteractiveForHandler{}
	handler := NewUserDashboardHandler(svc, chat, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, nil)

	w, c := newTestContextWithUser("POST", "/api/dashboard/tickets/OMS-1/transition", `{"transitionId":"11"}`)
	c.Params = gin.Params{{Key: "issueKey", Value: "OMS-1"}}
	handler.TransitionTicket(c)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"Code Process"`) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	if svc.transitionEmail != "dev@esb.co.id" || svc.transitionKey != "OMS-1" || svc.transitionIDCalled != "11" {
		t.Errorf("service got %q %q %q", svc.transitionEmail, svc.transitionKey, svc.transitionIDCalled)
	}
	if chat.lastCommand != "" {
		t.Errorf("a transitionId request must not go through RouteCommand, got %q", chat.lastCommand)
	}
}

func TestTransitionTicket_MapsErrors(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{user_dashboard_service.ErrNotOwnTicket, http.StatusForbidden},
		{user_dashboard_service.ErrTransitionUnavailable, http.StatusUnprocessableEntity},
		{errors.New("jira down"), http.StatusBadGateway},
	}
	for _, tc := range cases {
		svc := &fakeUserDashboardService{transitionErr: tc.err}
		handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, nil)
		w, c := newTestContextWithUser("POST", "/api/dashboard/tickets/OMS-1/transition", `{"transitionId":"11"}`)
		c.Params = gin.Params{{Key: "issueKey", Value: "OMS-1"}}
		handler.TransitionTicket(c)
		if w.Code != tc.want {
			t.Errorf("%v → %d, want %d", tc.err, w.Code, tc.want)
		}
	}
}

func TestTransitionTicket_EmptyBodyIs400(t *testing.T) {
	handler := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, nil)
	w, c := newTestContextWithUser("POST", "/api/dashboard/tickets/OMS-1/transition", `{}`)
	c.Params = gin.Params{{Key: "issueKey", Value: "OMS-1"}}
	handler.TransitionTicket(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGetTransitions_ReturnsListAndMapsErrors(t *testing.T) {
	svc := &fakeUserDashboardService{transitions: user_dashboard_dto.TicketTransitions{
		IssueKey: "OMS-1", CurrentStatus: "To Do",
		Transitions: []user_dashboard_dto.TransitionOption{{ID: "11", Name: "Start Development", ToStatus: "Code Process"}},
	}}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserSettingsForHandler{}, nil)
	w, c := newTestContextWithUser("GET", "/api/dashboard/tickets/OMS-1/transitions?email=victim@esb.co.id", "")
	c.Params = gin.Params{{Key: "issueKey", Value: "OMS-1"}}
	handler.GetTransitions(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"toStatus":"Code Process"`) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	if svc.transitionEmail != "dev@esb.co.id" {
		t.Errorf("email = %q, want the session user", svc.transitionEmail)
	}

	svc.transitionsErr = user_dashboard_service.ErrNotOwnTicket
	w, c = newTestContextWithUser("GET", "/api/dashboard/tickets/OMS-2/transitions", "")
	c.Params = gin.Params{{Key: "issueKey", Value: "OMS-2"}}
	handler.GetTransitions(c)
	if w.Code != http.StatusForbidden {
		t.Errorf("not own ticket → %d, want 403", w.Code)
	}
}

// Review finding #7: a board outside the user's active sprints that still
// has custom settings or a running mute must stay reachable from
// Pengaturan, otherwise it can never be reset.
func TestListBoards_ReportsOtherBoardsWithCustomSettingsOrMute(t *testing.T) {
	svc := &fakeUserDashboardService{
		boards:     []user_dashboard_dto.BoardSummary{{BoardID: 28, Name: "ESB OMS Board", ProjectKey: "OMS"}},
		boardNames: map[int64][2]string{40: {"Old Board", "OLD"}, 41: {"Muted Board", "MUT"}},
	}
	until := time.Now().Add(time.Hour)
	expired := time.Now().Add(-time.Hour)
	settings := &fakeUserSettingsForHandler{states: map[int64]user_setting_service.BoardState{
		28: {BoardID: 28, Custom: true},
		40: {BoardID: 40, Custom: true},
		41: {BoardID: 41, MutedUntil: &until},
		42: {BoardID: 42, MutedUntil: &expired}, // nothing left to undo
	}}
	handler := NewUserDashboardHandler(svc, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, settings, nil)

	w, c := newTestContextWithUser("GET", "/api/dashboard/boards", "")
	handler.ListBoards(c)

	var res struct {
		Boards      []map[string]any `json:"boards"`
		OtherBoards []struct {
			BoardID           int64  `json:"boardId"`
			Name              string `json:"name"`
			ProjectKey        string `json:"projectKey"`
			HasCustomSettings bool   `json:"hasCustomSettings"`
			Muted             struct {
				Active bool `json:"active"`
			} `json:"muted"`
		} `json:"otherBoards"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	if len(res.Boards) != 1 {
		t.Errorf("boards = %d, want 1 (the picker list is unchanged)", len(res.Boards))
	}
	if len(res.OtherBoards) != 2 {
		t.Fatalf("otherBoards = %+v, want boards 40 and 41", res.OtherBoards)
	}
	if o := res.OtherBoards[0]; o.BoardID != 40 || o.Name != "Old Board" || o.ProjectKey != "OLD" || !o.HasCustomSettings || o.Muted.Active {
		t.Errorf("otherBoards[0] = %+v", o)
	}
	if o := res.OtherBoards[1]; o.BoardID != 41 || o.HasCustomSettings || !o.Muted.Active {
		t.Errorf("otherBoards[1] = %+v", o)
	}
}
