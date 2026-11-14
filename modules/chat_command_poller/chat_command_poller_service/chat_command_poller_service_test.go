package chat_command_poller_service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nudge/modules/registration/registration_dto"
)

var errSendFailed = errors.New("send failed")

func newTestPoller(
	oauth *fakeChatOAuthServiceForPoller,
	registrationRepo *fakeRegistrationRepoForPoller,
	cursorRepo *fakeCursorRepoForPoller,
	chatInteractive *fakeChatInteractiveServiceForPoller,
	dmSender *fakeDMSenderForPoller,
	selfUserID string,
) *chatCommandPollerServiceImpl {
	return &chatCommandPollerServiceImpl{
		oauth:            oauth,
		registrationRepo: registrationRepo,
		cursorRepo:       cursorRepo,
		chatInteractive:  chatInteractive,
		dmSender:         dmSender,
		selfUserID:       selfUserID,
	}
}

// --- fakes ---

type fakeChatOAuthServiceForPoller struct {
	token string
	err   error
}

func (f *fakeChatOAuthServiceForPoller) GetAccessToken(ctx context.Context) (string, error) {
	return f.token, f.err
}

type fakeRegistrationRepoForPoller struct {
	users []registration_dto.User
}

func (f *fakeRegistrationRepoForPoller) UpsertUser(ctx context.Context, name, email, jiraAccountID string) error {
	return nil
}
func (f *fakeRegistrationRepoForPoller) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	return nil, false, nil
}
func (f *fakeRegistrationRepoForPoller) FindUserByJiraAccountID(ctx context.Context, jiraAccountID string) (*registration_dto.User, bool, error) {
	return nil, false, nil
}
func (f *fakeRegistrationRepoForPoller) UpdateGoogleChatDMSpaceID(ctx context.Context, email, spaceID string) error {
	return nil
}

func (f *fakeRegistrationRepoForPoller) UpdateReminderTierPreference(ctx context.Context, email, csv string) error {
	return nil
}

func (f *fakeRegistrationRepoForPoller) FindAll(ctx context.Context) ([]registration_dto.User, error) {
	return nil, nil
}

func (f *fakeRegistrationRepoForPoller) UpdateFlagActive(ctx context.Context, userID int64, active bool) error {
	return nil
}

func (f *fakeRegistrationRepoForPoller) UpdateJiraAccountID(ctx context.Context, userID int64, jiraAccountID string) error {
	return nil
}

func (f *fakeRegistrationRepoForPoller) DeleteByID(ctx context.Context, userID int64) error {
	return nil
}
func (f *fakeRegistrationRepoForPoller) FindAllWithGoogleChatDMSpaceID(ctx context.Context) ([]registration_dto.User, error) {
	return f.users, nil
}

type fakeCursorRepoForPoller struct {
	cursors      map[string]time.Time
	updateCalls  map[string]time.Time
	getCursorErr error
	// updateCallLog records every UpdateCursor call (in order), unlike
	// updateCalls which only keeps the latest per space — needed to assert
	// the cursor advances per-message, not once per batch.
	updateCallLog []time.Time
	// failUpdateAt, if > 0, makes the Nth UpdateCursor call (1-indexed,
	// across the whole test) fail and not persist — simulates a transient
	// DB write failure (or a process restart) partway through a batch.
	failUpdateAt int
}

func newFakeCursorRepoForPoller() *fakeCursorRepoForPoller {
	return &fakeCursorRepoForPoller{cursors: map[string]time.Time{}, updateCalls: map[string]time.Time{}}
}

func (f *fakeCursorRepoForPoller) GetCursor(ctx context.Context, spaceID string) (time.Time, bool, error) {
	if f.getCursorErr != nil {
		return time.Time{}, false, f.getCursorErr
	}
	t, ok := f.cursors[spaceID]
	return t, ok, nil
}

func (f *fakeCursorRepoForPoller) UpdateCursor(ctx context.Context, spaceID string, lastProcessed time.Time) error {
	f.updateCallLog = append(f.updateCallLog, lastProcessed)
	if f.failUpdateAt > 0 && len(f.updateCallLog) == f.failUpdateAt {
		return errors.New("simulated cursor persist failure")
	}
	f.updateCalls[spaceID] = lastProcessed
	f.cursors[spaceID] = lastProcessed
	return nil
}

type fakeChatInteractiveServiceForPoller struct {
	calls []string // "email|text"
	reply string
}

func (f *fakeChatInteractiveServiceForPoller) RouteCommand(ctx context.Context, senderEmail, messageText string) string {
	f.calls = append(f.calls, senderEmail+"|"+messageText)
	if f.reply != "" {
		return f.reply
	}
	return "routed reply"
}

type fakeDMSenderForPoller struct {
	calls   []string // "email|text"
	sendErr error
}

func (f *fakeDMSenderForPoller) SendPlainTextDM(ctx context.Context, email, text string) error {
	f.calls = append(f.calls, email+"|"+text)
	return f.sendErr
}

// --- test server helper ---

type messageFixture struct {
	senderName string
	text       string
	createTime time.Time
}

// newMessagesServer serves spaces.messages.list responses keyed by the
// space segment of the request path (e.g. "spaces/ABC" -> its fixtures),
// mirroring the real Chat API's per-space endpoint shape.
func newMessagesServer(t *testing.T, bySpace map[string][]messageFixture, errSpaces map[string]int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// path shape: /{space}/messages, space may itself contain a slash
		// (e.g. "spaces/ABC"), so trim the known "/messages" suffix instead
		// of splitting on "/".
		space := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/messages"), "/")
		if code, bad := errSpaces[space]; bad {
			w.WriteHeader(code)
			return
		}
		fixtures := bySpace[space]
		type msg struct {
			Name   string `json:"name"`
			Sender struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"sender"`
			CreateTime string `json:"createTime"`
			Text       string `json:"text"`
		}
		resp := struct {
			Messages []msg `json:"messages"`
		}{}
		for i, f := range fixtures {
			var m msg
			m.Name = space + "/messages/" + string(rune('a'+i))
			m.Sender.Name = f.senderName
			m.Sender.Type = "HUMAN"
			m.CreateTime = f.createTime.Format(time.RFC3339Nano)
			m.Text = f.text
			resp.Messages = append(resp.Messages, m)
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

func withTestServer(server *httptest.Server, fn func()) {
	original := chatAPIBase
	chatAPIBase = server.URL
	defer func() { chatAPIBase = original }()
	fn()
}

const selfUserID = "users/SELF123"

// --- tests ---

func TestPollOnce_RepliesToRecognizedCommandAfterCursor(t *testing.T) {
	cursor := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	msgTime := cursor.Add(1 * time.Minute)
	server := newMessagesServer(t, map[string][]messageFixture{
		"spaces/ABC": {{senderName: "users/OTHER", text: "/n-mytickets", createTime: msgTime}},
	}, nil)
	defer server.Close()

	cursorRepo := newFakeCursorRepoForPoller()
	cursorRepo.cursors["spaces/ABC"] = cursor
	chatInteractive := &fakeChatInteractiveServiceForPoller{reply: "here are your tickets"}
	dmSender := &fakeDMSenderForPoller{}
	registrationRepo := &fakeRegistrationRepoForPoller{users: []registration_dto.User{
		{Email: "teammate@esb.co.id", GoogleChatDMSpaceID: "spaces/ABC"},
	}}
	svc := newTestPoller(&fakeChatOAuthServiceForPoller{token: "tok"}, registrationRepo, cursorRepo, chatInteractive, dmSender, selfUserID)

	withTestServer(server, func() {
		if err := svc.pollOnce(context.Background()); err != nil {
			t.Fatalf("pollOnce returned error: %v", err)
		}
	})

	if len(chatInteractive.calls) != 1 || chatInteractive.calls[0] != "teammate@esb.co.id|/n-mytickets" {
		t.Fatalf("RouteCommand calls = %v, want [teammate@esb.co.id|/n-mytickets]", chatInteractive.calls)
	}
	if len(dmSender.calls) != 1 || dmSender.calls[0] != "teammate@esb.co.id|🤖 here are your tickets" {
		t.Fatalf("SendPlainTextDM calls = %v, want disclosure-marker-prefixed reply", dmSender.calls)
	}
	if !cursorRepo.updateCalls["spaces/ABC"].Equal(msgTime) {
		t.Errorf("cursor updated to %v, want %v", cursorRepo.updateCalls["spaces/ABC"], msgTime)
	}
}

func TestPollOnce_IgnoresPlainConversationText(t *testing.T) {
	cursor := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	msgTime := cursor.Add(1 * time.Minute)
	server := newMessagesServer(t, map[string][]messageFixture{
		"spaces/ABC": {{senderName: "users/OTHER", text: "ahahahha task 4 jam dikerjakan 2 jam", createTime: msgTime}},
	}, nil)
	defer server.Close()

	cursorRepo := newFakeCursorRepoForPoller()
	cursorRepo.cursors["spaces/ABC"] = cursor
	chatInteractive := &fakeChatInteractiveServiceForPoller{}
	dmSender := &fakeDMSenderForPoller{}
	registrationRepo := &fakeRegistrationRepoForPoller{users: []registration_dto.User{
		{Email: "teammate@esb.co.id", GoogleChatDMSpaceID: "spaces/ABC"},
	}}
	svc := newTestPoller(&fakeChatOAuthServiceForPoller{token: "tok"}, registrationRepo, cursorRepo, chatInteractive, dmSender, selfUserID)

	withTestServer(server, func() {
		if err := svc.pollOnce(context.Background()); err != nil {
			t.Fatalf("pollOnce returned error: %v", err)
		}
	})

	if len(chatInteractive.calls) != 0 {
		t.Errorf("RouteCommand calls = %v, want none — plain chat text must never trigger a command", chatInteractive.calls)
	}
	if len(dmSender.calls) != 0 {
		t.Errorf("SendPlainTextDM calls = %v, want none", dmSender.calls)
	}
	// Cursor still advances so this message is never re-inspected.
	if !cursorRepo.updateCalls["spaces/ABC"].Equal(msgTime) {
		t.Errorf("cursor updated to %v, want %v (must still advance past ignored messages)", cursorRepo.updateCalls["spaces/ABC"], msgTime)
	}
}

func TestPollOnce_SkipsOwnMessagesEvenIfLookLikeCommands(t *testing.T) {
	cursor := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	msgTime := cursor.Add(1 * time.Minute)
	server := newMessagesServer(t, map[string][]messageFixture{
		"spaces/ABC": {{senderName: selfUserID, text: "/n-mytickets", createTime: msgTime}},
	}, nil)
	defer server.Close()

	cursorRepo := newFakeCursorRepoForPoller()
	cursorRepo.cursors["spaces/ABC"] = cursor
	chatInteractive := &fakeChatInteractiveServiceForPoller{}
	dmSender := &fakeDMSenderForPoller{}
	registrationRepo := &fakeRegistrationRepoForPoller{users: []registration_dto.User{
		{Email: "teammate@esb.co.id", GoogleChatDMSpaceID: "spaces/ABC"},
	}}
	svc := newTestPoller(&fakeChatOAuthServiceForPoller{token: "tok"}, registrationRepo, cursorRepo, chatInteractive, dmSender, selfUserID)

	withTestServer(server, func() {
		if err := svc.pollOnce(context.Background()); err != nil {
			t.Fatalf("pollOnce returned error: %v", err)
		}
	})

	if len(chatInteractive.calls) != 0 {
		t.Errorf("RouteCommand calls = %v, want none — must never react to its own sent messages", chatInteractive.calls)
	}
}

func TestPollOnce_SkipsMessagesAtOrBeforeCursor(t *testing.T) {
	cursor := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	server := newMessagesServer(t, map[string][]messageFixture{
		"spaces/ABC": {
			{senderName: "users/OTHER", text: "/n-mytickets", createTime: cursor},                       // exactly at cursor
			{senderName: "users/OTHER", text: "/n-mytickets", createTime: cursor.Add(-1 * time.Minute)}, // before cursor
		},
	}, nil)
	defer server.Close()

	cursorRepo := newFakeCursorRepoForPoller()
	cursorRepo.cursors["spaces/ABC"] = cursor
	chatInteractive := &fakeChatInteractiveServiceForPoller{}
	dmSender := &fakeDMSenderForPoller{}
	registrationRepo := &fakeRegistrationRepoForPoller{users: []registration_dto.User{
		{Email: "teammate@esb.co.id", GoogleChatDMSpaceID: "spaces/ABC"},
	}}
	svc := newTestPoller(&fakeChatOAuthServiceForPoller{token: "tok"}, registrationRepo, cursorRepo, chatInteractive, dmSender, selfUserID)

	withTestServer(server, func() {
		if err := svc.pollOnce(context.Background()); err != nil {
			t.Fatalf("pollOnce returned error: %v", err)
		}
	})

	if len(chatInteractive.calls) != 0 {
		t.Errorf("RouteCommand calls = %v, want none — both messages are at/before the stored cursor", chatInteractive.calls)
	}
}

func TestPollOnce_FirstPollForSpaceDoesNotReplayHistory(t *testing.T) {
	msgTime := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	server := newMessagesServer(t, map[string][]messageFixture{
		"spaces/ABC": {{senderName: "users/OTHER", text: "/n-mytickets", createTime: msgTime}},
	}, nil)
	defer server.Close()

	cursorRepo := newFakeCursorRepoForPoller() // no cursor stored yet for spaces/ABC
	chatInteractive := &fakeChatInteractiveServiceForPoller{}
	dmSender := &fakeDMSenderForPoller{}
	registrationRepo := &fakeRegistrationRepoForPoller{users: []registration_dto.User{
		{Email: "teammate@esb.co.id", GoogleChatDMSpaceID: "spaces/ABC"},
	}}
	svc := newTestPoller(&fakeChatOAuthServiceForPoller{token: "tok"}, registrationRepo, cursorRepo, chatInteractive, dmSender, selfUserID)

	withTestServer(server, func() {
		if err := svc.pollOnce(context.Background()); err != nil {
			t.Fatalf("pollOnce returned error: %v", err)
		}
	})

	if len(chatInteractive.calls) != 0 {
		t.Errorf("RouteCommand calls = %v, want none — first-ever poll must not replay pre-existing history", chatInteractive.calls)
	}
	if !cursorRepo.updateCalls["spaces/ABC"].Equal(msgTime) {
		t.Errorf("cursor initialized to %v, want %v so future cycles start from here", cursorRepo.updateCalls["spaces/ABC"], msgTime)
	}
}

func TestPollOnce_ListMessagesErrorSkipsSpaceCursorUnchanged(t *testing.T) {
	cursor := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	server := newMessagesServer(t, nil, map[string]int{"spaces/ABC": http.StatusInternalServerError})
	defer server.Close()

	cursorRepo := newFakeCursorRepoForPoller()
	cursorRepo.cursors["spaces/ABC"] = cursor
	registrationRepo := &fakeRegistrationRepoForPoller{users: []registration_dto.User{
		{Email: "teammate@esb.co.id", GoogleChatDMSpaceID: "spaces/ABC"},
	}}
	svc := newTestPoller(&fakeChatOAuthServiceForPoller{token: "tok"}, registrationRepo, cursorRepo, &fakeChatInteractiveServiceForPoller{}, &fakeDMSenderForPoller{}, selfUserID)

	withTestServer(server, func() {
		if err := svc.pollOnce(context.Background()); err != nil {
			t.Fatalf("pollOnce returned error: %v, want nil — a single space's failure must not fail the whole cycle", err)
		}
	})

	if _, updated := cursorRepo.updateCalls["spaces/ABC"]; updated {
		t.Error("cursor must not advance when listing messages failed")
	}
}

func TestPollOnce_SendReplyErrorStillAdvancesCursor(t *testing.T) {
	cursor := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	msgTime := cursor.Add(1 * time.Minute)
	server := newMessagesServer(t, map[string][]messageFixture{
		"spaces/ABC": {{senderName: "users/OTHER", text: "/n-overdue", createTime: msgTime}},
	}, nil)
	defer server.Close()

	cursorRepo := newFakeCursorRepoForPoller()
	cursorRepo.cursors["spaces/ABC"] = cursor
	registrationRepo := &fakeRegistrationRepoForPoller{users: []registration_dto.User{
		{Email: "teammate@esb.co.id", GoogleChatDMSpaceID: "spaces/ABC"},
	}}
	dmSender := &fakeDMSenderForPoller{sendErr: errSendFailed}
	svc := newTestPoller(&fakeChatOAuthServiceForPoller{token: "tok"}, registrationRepo, cursorRepo, &fakeChatInteractiveServiceForPoller{}, dmSender, selfUserID)

	withTestServer(server, func() {
		if err := svc.pollOnce(context.Background()); err != nil {
			t.Fatalf("pollOnce returned error: %v, want nil — a send failure must be logged, not fail the cycle", err)
		}
	})

	if !cursorRepo.updateCalls["spaces/ABC"].Equal(msgTime) {
		t.Errorf("cursor = %v, want %v — must still advance even though the reply failed to send (avoids retry-looping the same message)", cursorRepo.updateCalls["spaces/ABC"], msgTime)
	}
}

func TestPollOnce_ProcessesEachUsersSpaceIndependently(t *testing.T) {
	cursor := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	msgTime := cursor.Add(1 * time.Minute)
	server := newMessagesServer(t, map[string][]messageFixture{
		"spaces/ABC": {{senderName: "users/OTHER1", text: "/n-mytickets", createTime: msgTime}},
		"spaces/XYZ": {{senderName: "users/OTHER2", text: "/n-team", createTime: msgTime}},
	}, nil)
	defer server.Close()

	cursorRepo := newFakeCursorRepoForPoller()
	cursorRepo.cursors["spaces/ABC"] = cursor
	cursorRepo.cursors["spaces/XYZ"] = cursor
	chatInteractive := &fakeChatInteractiveServiceForPoller{}
	registrationRepo := &fakeRegistrationRepoForPoller{users: []registration_dto.User{
		{Email: "teammate1@esb.co.id", GoogleChatDMSpaceID: "spaces/ABC"},
		{Email: "teammate2@esb.co.id", GoogleChatDMSpaceID: "spaces/XYZ"},
	}}
	svc := newTestPoller(&fakeChatOAuthServiceForPoller{token: "tok"}, registrationRepo, cursorRepo, chatInteractive, &fakeDMSenderForPoller{}, selfUserID)

	withTestServer(server, func() {
		if err := svc.pollOnce(context.Background()); err != nil {
			t.Fatalf("pollOnce returned error: %v", err)
		}
	})

	if len(chatInteractive.calls) != 2 {
		t.Fatalf("RouteCommand calls = %v, want 2 (one per user's space)", chatInteractive.calls)
	}
}

func TestPollOnce_AdvancesCursorPerMessageNotOncePerBatch(t *testing.T) {
	cursor := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	msg1Time := cursor.Add(1 * time.Minute)
	msg2Time := cursor.Add(2 * time.Minute)
	server := newMessagesServer(t, map[string][]messageFixture{
		"spaces/ABC": {
			{senderName: "users/OTHER", text: "/n-geser<CORE-1>:code-review", createTime: msg1Time},
			{senderName: "users/OTHER", text: "/n-geser<CORE-2>:code-review", createTime: msg2Time},
		},
	}, nil)
	defer server.Close()

	cursorRepo := newFakeCursorRepoForPoller()
	cursorRepo.cursors["spaces/ABC"] = cursor
	chatInteractive := &fakeChatInteractiveServiceForPoller{}
	dmSender := &fakeDMSenderForPoller{}
	registrationRepo := &fakeRegistrationRepoForPoller{users: []registration_dto.User{
		{Email: "teammate@esb.co.id", GoogleChatDMSpaceID: "spaces/ABC"},
	}}
	svc := newTestPoller(&fakeChatOAuthServiceForPoller{token: "tok"}, registrationRepo, cursorRepo, chatInteractive, dmSender, selfUserID)

	withTestServer(server, func() {
		if err := svc.pollOnce(context.Background()); err != nil {
			t.Fatalf("pollOnce returned error: %v", err)
		}
	})

	if len(cursorRepo.updateCallLog) != 2 {
		t.Fatalf("UpdateCursor call count = %d, want 2 — the cursor must advance after EACH message, not once at the end of the batch (a crash/restart between two replies would otherwise replay the whole batch and double-send, reproducing a real production incident)", len(cursorRepo.updateCallLog))
	}
	if !cursorRepo.updateCallLog[0].Equal(msg1Time) || !cursorRepo.updateCallLog[1].Equal(msg2Time) {
		t.Errorf("UpdateCursor call sequence = %v, want [%v, %v] in order", cursorRepo.updateCallLog, msg1Time, msg2Time)
	}
}

func TestPollOnce_CursorPersistFailureOnlyReplaysUnpersistedTail(t *testing.T) {
	cursor := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	msg1Time := cursor.Add(1 * time.Minute)
	msg2Time := cursor.Add(2 * time.Minute)
	server := newMessagesServer(t, map[string][]messageFixture{
		"spaces/ABC": {
			{senderName: "users/OTHER", text: "/n-geser<CORE-1>:code-review", createTime: msg1Time},
			{senderName: "users/OTHER", text: "/n-geser<CORE-2>:code-review", createTime: msg2Time},
		},
	}, nil)
	defer server.Close()

	cursorRepo := newFakeCursorRepoForPoller()
	cursorRepo.cursors["spaces/ABC"] = cursor
	cursorRepo.failUpdateAt = 2 // simulate msg2's cursor write failing (transient DB hiccup / restart)
	chatInteractive := &fakeChatInteractiveServiceForPoller{}
	dmSender := &fakeDMSenderForPoller{}
	registrationRepo := &fakeRegistrationRepoForPoller{users: []registration_dto.User{
		{Email: "teammate@esb.co.id", GoogleChatDMSpaceID: "spaces/ABC"},
	}}
	svc := newTestPoller(&fakeChatOAuthServiceForPoller{token: "tok"}, registrationRepo, cursorRepo, chatInteractive, dmSender, selfUserID)

	withTestServer(server, func() {
		if err := svc.pollOnce(context.Background()); err != nil {
			t.Fatalf("first pollOnce returned error: %v", err)
		}
	})
	if len(chatInteractive.calls) != 2 {
		t.Fatalf("after first pollOnce, RouteCommand calls = %v, want 2 (both messages replied to once)", chatInteractive.calls)
	}

	cursorRepo.failUpdateAt = 0 // the transient failure is over now (e.g. service back up)
	withTestServer(server, func() {
		if err := svc.pollOnce(context.Background()); err != nil {
			t.Fatalf("second pollOnce returned error: %v", err)
		}
	})

	if len(chatInteractive.calls) != 3 {
		t.Fatalf("after second pollOnce, RouteCommand calls = %v, want 3 total — only msg2 (whose cursor write failed) should replay, msg1's cursor was already safely persisted before the failure", chatInteractive.calls)
	}
	if chatInteractive.calls[2] != "teammate@esb.co.id|/n-geser<CORE-2>:code-review" {
		t.Errorf("the replayed call = %q, want it to be the CORE-2 message", chatInteractive.calls[2])
	}
}
