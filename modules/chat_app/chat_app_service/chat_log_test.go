package chat_app_service

import (
	"context"
	"sync"
	"testing"
	"time"

	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/chat_log/chat_log_dto"
	"nudge/modules/registration/registration_dto"
)

type fakeRecorder struct {
	mu      sync.Mutex
	entries []chat_log_dto.Entry
}

func (f *fakeRecorder) Record(_ context.Context, e chat_log_dto.Entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
}

func (f *fakeRecorder) all() []chat_log_dto.Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]chat_log_dto.Entry(nil), f.entries...)
}

func loggedSvc(rec *fakeRecorder, cmds CommandRouter, app AppSender, timeout time.Duration) Service {
	return New(Deps{
		Users:          fakeUsers{dev: {Email: dev, FlagActive: true}},
		States:         &fakeStates{states: map[string]chat_app_dto.State{dev: {SpaceID: "spaces/DM"}}},
		Commands:       cmds,
		App:            app,
		CommandTimeout: timeout,
		Log:            rec,
	})
}

func TestLog_CommandAndReplyAreRecorded(t *testing.T) {
	rec := &fakeRecorder{}
	svc := loggedSvc(rec, &fakeCommands{}, nil, 0)
	svc.Handle(context.Background(), dmEvent("MESSAGE", dev, &chat_app_dto.Message{Text: "/n-mytickets"}))

	got := rec.all()
	if len(got) != 2 {
		t.Fatalf("entries = %+v", got)
	}
	in, out := got[0], got[1]
	if in.Direction != "in" || in.Kind != "command" || in.Body != "/n-mytickets" || in.Status != "received" || in.Email != dev {
		t.Errorf("in = %+v", in)
	}
	if out.Direction != "out" || out.Kind != "reply" || out.Channel != "app" || out.Body != "reply to /n-mytickets" {
		t.Errorf("out = %+v", out)
	}
}

func TestLog_AddedAndRemovedAreRecorded(t *testing.T) {
	rec := &fakeRecorder{}
	svc := loggedSvc(rec, &fakeCommands{}, nil, 0)
	svc.Handle(context.Background(), dmEvent("ADDED_TO_SPACE", dev, nil))
	svc.Handle(context.Background(), dmEvent("REMOVED_FROM_SPACE", dev, nil))

	got := rec.all()
	// added (in) + welcome (out) + removed (in; no reply).
	if len(got) != 3 || got[0].Kind != "added" || got[1].Kind != "reply" || got[2].Kind != "removed" {
		t.Fatalf("entries = %+v", got)
	}
}

func TestLog_CardClickIsRecorded(t *testing.T) {
	rec := &fakeRecorder{}
	g := &fakeGeser{}
	svc := New(Deps{
		Users: fakeUsers{dev: {Email: dev, FlagActive: true}}, States: &fakeStates{states: map[string]chat_app_dto.State{}},
		Commands: g, Log: rec,
	})
	svc.Handle(context.Background(), clickEvent(dev))
	got := rec.all()
	if len(got) == 0 || got[0].Kind != "click" || got[0].Direction != "in" {
		t.Fatalf("entries = %+v", got)
	}
}

func TestLog_LateReplyIsRecorded(t *testing.T) {
	rec := &fakeRecorder{}
	app := &fakeApp{sent: make(chan chat_app_dto.OutgoingMessage, 1)}
	slow := &fakeCommands{delay: 50 * time.Millisecond}
	svc := loggedSvc(rec, slow, app, 5*time.Millisecond)
	svc.Handle(context.Background(), dmEvent("MESSAGE", dev, &chat_app_dto.Message{Text: "/n-team"}))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range rec.all() {
			if e.Kind == "reply" && e.Body == "reply to /n-team" && e.Status == "sent" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("late reply not recorded: %+v", rec.all())
}

func TestLog_NilRecorderIsFine(t *testing.T) {
	svc := New(Deps{Users: fakeUsers{dev: registration_dto.User{Email: dev, FlagActive: true}}, States: &fakeStates{states: map[string]chat_app_dto.State{}}, Commands: &fakeCommands{}})
	svc.Handle(context.Background(), dmEvent("MESSAGE", dev, &chat_app_dto.Message{Text: "halo"}))
}
