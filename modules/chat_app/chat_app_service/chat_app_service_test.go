package chat_app_service

import (
	"context"
	"strings"
	"testing"
	"time"

	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/registration/registration_dto"
)

type fakeUsers map[string]registration_dto.User

func (f fakeUsers) FindUserByEmail(_ context.Context, email string) (*registration_dto.User, bool, error) {
	u, ok := f[email]
	if !ok {
		return nil, false, nil
	}
	return &u, true, nil
}

type fakeStates struct {
	states  map[string]chat_app_dto.State
	set     []string
	cleared []string
}

func (f *fakeStates) FindByEmail(_ context.Context, email string) (chat_app_dto.State, bool, error) {
	s, ok := f.states[email]
	return s, ok, nil
}
func (f *fakeStates) SetSpace(_ context.Context, email, space, chatUser string) error {
	f.set = append(f.set, email+"|"+space+"|"+chatUser)
	return nil
}
func (f *fakeStates) ClearBySpace(_ context.Context, space string) error {
	f.cleared = append(f.cleared, space)
	return nil
}

type fakeCommands struct {
	delay time.Duration
	got   []string
}

func (f *fakeCommands) RouteCommand(_ context.Context, email, text string) string {
	time.Sleep(f.delay)
	f.got = append(f.got, email+"|"+text)
	return "reply to " + text
}

type fakeApp struct {
	sent chan chat_app_dto.OutgoingMessage
}

func (f *fakeApp) CreateMessage(_ context.Context, space string, msg chat_app_dto.OutgoingMessage) error {
	f.sent <- msg
	return nil
}

const dev = "dev@esb.co.id"

func newSvc(states *fakeStates, cmds *fakeCommands, app *fakeApp) Service {
	return New(Deps{
		Users:    fakeUsers{dev: {Email: dev, FlagActive: true}, "off@esb.co.id": {Email: "off@esb.co.id", FlagActive: false}},
		States:   states,
		Commands: cmds,
		App:      app,
	})
}

func dmEvent(typ, email string, msg *chat_app_dto.Message) chat_app_dto.Event {
	return chat_app_dto.Event{
		Type:    typ,
		Space:   chat_app_dto.Space{Name: "spaces/DM", Type: "DM"},
		User:    chat_app_dto.ChatUser{Name: "users/9", Email: email, DisplayName: "Dev"},
		Message: msg,
	}
}

func TestHandle_AddedRegisteredStoresSpaceAndWelcomes(t *testing.T) {
	states := &fakeStates{states: map[string]chat_app_dto.State{}}
	res := newSvc(states, &fakeCommands{}, nil).Handle(context.Background(), dmEvent("ADDED_TO_SPACE", dev, nil))
	if len(states.set) != 1 || states.set[0] != dev+"|spaces/DM|users/9" {
		t.Errorf("SetSpace calls = %v", states.set)
	}
	if !strings.Contains(res.Text, "/n-help") {
		t.Errorf("welcome = %q", res.Text)
	}
}

func TestHandle_AddedUnknownOrInactiveIsNotStored(t *testing.T) {
	states := &fakeStates{states: map[string]chat_app_dto.State{}}
	svc := newSvc(states, &fakeCommands{}, nil)
	res := svc.Handle(context.Background(), dmEvent("ADDED_TO_SPACE", "stranger@esb.co.id", nil))
	if !strings.Contains(res.Text, "belum terdaftar") {
		t.Errorf("unknown reply = %q", res.Text)
	}
	res = svc.Handle(context.Background(), dmEvent("ADDED_TO_SPACE", "off@esb.co.id", nil))
	if !strings.Contains(res.Text, "dinonaktifkan") {
		t.Errorf("inactive reply = %q", res.Text)
	}
	if len(states.set) != 0 {
		t.Errorf("SetSpace must not run: %v", states.set)
	}
}

func TestHandle_RemovedClearsBySpace(t *testing.T) {
	states := &fakeStates{states: map[string]chat_app_dto.State{}}
	res := newSvc(states, &fakeCommands{}, nil).Handle(context.Background(), dmEvent("REMOVED_FROM_SPACE", dev, nil))
	if len(states.cleared) != 1 || states.cleared[0] != "spaces/DM" || res.Text != "" {
		t.Errorf("cleared=%v res=%+v", states.cleared, res)
	}
}

func TestHandle_MessageRoutesCommandAndStoresMissingSpace(t *testing.T) {
	states := &fakeStates{states: map[string]chat_app_dto.State{dev: {Email: dev}}}
	cmds := &fakeCommands{}
	res := newSvc(states, cmds, nil).Handle(context.Background(), dmEvent("MESSAGE", dev, &chat_app_dto.Message{Text: " /n-mytickets ", ArgumentText: "/n-mytickets"}))
	if res.Text != "reply to /n-mytickets" || cmds.got[0] != dev+"|/n-mytickets" {
		t.Errorf("res=%q got=%v", res.Text, cmds.got)
	}
	if len(states.set) != 1 {
		t.Errorf("missing space should be stored: %v", states.set)
	}
}

func TestHandle_MessageWithKnownSpaceDoesNotRewrite(t *testing.T) {
	states := &fakeStates{states: map[string]chat_app_dto.State{dev: {Email: dev, SpaceID: "spaces/DM"}}}
	newSvc(states, &fakeCommands{}, nil).Handle(context.Background(), dmEvent("MESSAGE", dev, &chat_app_dto.Message{Text: "/n-help"}))
	if len(states.set) != 0 {
		t.Errorf("SetSpace = %v, want none", states.set)
	}
}

func TestHandle_SlowCommandRepliesLaterThroughTheApp(t *testing.T) {
	states := &fakeStates{states: map[string]chat_app_dto.State{dev: {Email: dev, SpaceID: "spaces/DM"}}}
	app := &fakeApp{sent: make(chan chat_app_dto.OutgoingMessage, 1)}
	svc := New(Deps{
		Users: fakeUsers{dev: {Email: dev, FlagActive: true}}, States: states,
		Commands: &fakeCommands{delay: 80 * time.Millisecond}, App: app, CommandTimeout: 10 * time.Millisecond,
	})
	res := svc.Handle(context.Background(), dmEvent("MESSAGE", dev, &chat_app_dto.Message{Text: "/n-team"}))
	if res.Text != processingText {
		t.Fatalf("immediate reply = %q, want %q", res.Text, processingText)
	}
	select {
	case msg := <-app.sent:
		if msg.Text != "reply to /n-team" {
			t.Errorf("late reply = %q", msg.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("no late reply sent")
	}
}

func TestHandle_NonDMIsIgnored(t *testing.T) {
	ev := dmEvent("MESSAGE", dev, &chat_app_dto.Message{Text: "/n-help"})
	ev.Space = chat_app_dto.Space{Name: "spaces/ROOM", Type: "ROOM"}
	res := newSvc(&fakeStates{states: map[string]chat_app_dto.State{}}, &fakeCommands{}, nil).Handle(context.Background(), ev)
	if res.Text != "" {
		t.Errorf("non-DM reply = %+v", res)
	}
}
