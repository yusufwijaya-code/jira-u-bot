package chat_dm_notifier_service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_dto"
	"jira-u-bot/modules/registration/registration_dto"
)

type fakeChatOAuthService struct {
	token string
	err   error
}

func (f *fakeChatOAuthService) GetAccessToken(ctx context.Context) (string, error) {
	return f.token, f.err
}

type fakeRegistrationRepositoryForDM struct {
	users              map[string]registration_dto.User
	updateSpaceIDCalls map[string]string
}

func newFakeRegistrationRepositoryForDM() *fakeRegistrationRepositoryForDM {
	return &fakeRegistrationRepositoryForDM{
		users:              map[string]registration_dto.User{},
		updateSpaceIDCalls: map[string]string{},
	}
}

func (f *fakeRegistrationRepositoryForDM) UpsertUser(ctx context.Context, name, email, jiraAccountID string) error {
	return nil
}

func (f *fakeRegistrationRepositoryForDM) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	u, ok := f.users[email]
	if !ok {
		return nil, false, nil
	}
	cp := u
	return &cp, true, nil
}

func (f *fakeRegistrationRepositoryForDM) FindUserByJiraAccountID(ctx context.Context, jiraAccountID string) (*registration_dto.User, bool, error) {
	return nil, false, nil
}

func (f *fakeRegistrationRepositoryForDM) UpdateGoogleChatDMSpaceID(ctx context.Context, email, spaceID string) error {
	f.updateSpaceIDCalls[email] = spaceID
	u := f.users[email]
	u.GoogleChatDMSpaceID = spaceID
	f.users[email] = u
	return nil
}

func TestResolveDMSpace_CallsSpacesSetupWhenNotCached(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(map[string]string{"name": "spaces/NEWSPACE123"})
	}))
	defer server.Close()
	originalBase := chatAPIBase
	chatAPIBase = server.URL
	defer func() { chatAPIBase = originalBase }()

	repo := newFakeRegistrationRepositoryForDM()
	repo.users["teammate@esb.co.id"] = registration_dto.User{Email: "teammate@esb.co.id"}
	svc := NewChatDMNotifierService(&fakeChatOAuthService{token: "fake-token"}, repo)
	impl := svc.(*chatDMNotifierServiceImpl)

	spaceID, err := impl.resolveDMSpace(context.Background(), "teammate@esb.co.id")
	if err != nil {
		t.Fatalf("resolveDMSpace returned error: %v", err)
	}
	if spaceID != "spaces/NEWSPACE123" {
		t.Errorf("spaceID = %q, want %q", spaceID, "spaces/NEWSPACE123")
	}
	if gotPath != "/spaces:setup" {
		t.Errorf("request path = %q, want %q", gotPath, "/spaces:setup")
	}
	if repo.updateSpaceIDCalls["teammate@esb.co.id"] != "spaces/NEWSPACE123" {
		t.Errorf("expected UpdateGoogleChatDMSpaceID to cache the resolved space ID, got calls: %v", repo.updateSpaceIDCalls)
	}
}

func TestResolveDMSpace_SkipsSpacesSetupWhenAlreadyCached(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		json.NewEncoder(w).Encode(map[string]string{"name": "spaces/SHOULDNOTHAPPEN"})
	}))
	defer server.Close()
	originalBase := chatAPIBase
	chatAPIBase = server.URL
	defer func() { chatAPIBase = originalBase }()

	repo := newFakeRegistrationRepositoryForDM()
	repo.users["teammate2@esb.co.id"] = registration_dto.User{Email: "teammate2@esb.co.id", GoogleChatDMSpaceID: "spaces/CACHED456"}
	svc := NewChatDMNotifierService(&fakeChatOAuthService{token: "fake-token"}, repo)
	impl := svc.(*chatDMNotifierServiceImpl)

	spaceID, err := impl.resolveDMSpace(context.Background(), "teammate2@esb.co.id")
	if err != nil {
		t.Fatalf("resolveDMSpace returned error: %v", err)
	}
	if spaceID != "spaces/CACHED456" {
		t.Errorf("spaceID = %q, want cached value %q", spaceID, "spaces/CACHED456")
	}
	if callCount != 0 {
		t.Errorf("spaces:setup was called %d times, want 0 (spec AC2.4 — cached value must skip the round trip)", callCount)
	}
}

func TestResolveDMSpace_PropagatesSpacesSetupError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	originalBase := chatAPIBase
	chatAPIBase = server.URL
	defer func() { chatAPIBase = originalBase }()

	repo := newFakeRegistrationRepositoryForDM()
	svc := NewChatDMNotifierService(&fakeChatOAuthService{token: "fake-token"}, repo)
	impl := svc.(*chatDMNotifierServiceImpl)

	_, err := impl.resolveDMSpace(context.Background(), "unregistered@esb.co.id")
	if err == nil {
		t.Fatal("expected error to propagate when spaces:setup returns a non-2xx status")
	}
}

func TestSendReminder_ResolvesSpaceAndPostsFormattedMessage(t *testing.T) {
	var gotPath, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/spaces:setup" {
			json.NewEncoder(w).Encode(map[string]string{"name": "spaces/XYZ"})
			return
		}
		gotPath = r.URL.Path
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		gotBody = body["text"]
		json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer server.Close()
	originalBase := chatAPIBase
	chatAPIBase = server.URL
	defer func() { chatAPIBase = originalBase }()

	repo := newFakeRegistrationRepositoryForDM()
	repo.users["teammate@esb.co.id"] = registration_dto.User{Email: "teammate@esb.co.id"}
	svc := NewChatDMNotifierService(&fakeChatOAuthService{token: "fake-token"}, repo)

	msg := google_chat_notifier_dto.ReminderMessage{
		IssueKey:       "OMS-1",
		IssueSummary:   "Test ticket",
		AssigneeName:   "Teammate",
		AssigneeEmail:  "teammate@esb.co.id",
		StoryPoints:    3,
		BudgetSeconds:  10800,
		ElapsedSeconds: 9000,
		Tier:           "30m",
	}

	if err := svc.SendReminder(context.Background(), msg); err != nil {
		t.Fatalf("SendReminder returned error: %v", err)
	}
	if gotPath != "/spaces/XYZ/messages" {
		t.Errorf("message POST path = %q, want %q", gotPath, "/spaces/XYZ/messages")
	}
	if !strings.Contains(gotBody, "OMS-1") {
		t.Errorf("message text %q does not contain issue key", gotBody)
	}
}

func TestSendPlainTextDM_PostsRawTextToResolvedSpace(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/spaces:setup" {
			json.NewEncoder(w).Encode(map[string]string{"name": "spaces/ABC"})
			return
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		gotBody = body["text"]
		json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer server.Close()
	originalBase := chatAPIBase
	chatAPIBase = server.URL
	defer func() { chatAPIBase = originalBase }()

	repo := newFakeRegistrationRepositoryForDM()
	repo.users["newuser@esb.co.id"] = registration_dto.User{Email: "newuser@esb.co.id"}
	svc := NewChatDMNotifierService(&fakeChatOAuthService{token: "fake-token"}, repo)

	if err := svc.SendPlainTextDM(context.Background(), "newuser@esb.co.id", "Terima kasih sudah mendaftar!"); err != nil {
		t.Fatalf("SendPlainTextDM returned error: %v", err)
	}
	if gotBody != "Terima kasih sudah mendaftar!" {
		t.Errorf("message text = %q, want %q", gotBody, "Terima kasih sudah mendaftar!")
	}
}
