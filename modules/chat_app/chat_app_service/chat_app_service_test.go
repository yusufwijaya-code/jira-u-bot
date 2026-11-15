package chat_app_service

import (
	"context"
	"strings"
	"testing"
	"time"

	"nudge/modules/chat_app/chat_app_card"
	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/chat_interactive/chat_interactive_service"
	"nudge/modules/google_chat_notifier/google_chat_notifier_dto"
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

func TestCommandText_SlashCommands(t *testing.T) {
	slash := func(id, args string) *chat_app_dto.Message {
		return &chat_app_dto.Message{Text: "/x " + args, ArgumentText: args, SlashCommand: &chat_app_dto.SlashCommand{CommandID: chat_app_dto.FlexString(id)}}
	}
	cases := []struct {
		msg        *chat_app_dto.Message
		want, hint string
	}{
		{slash("1", ""), "/n-mytickets", ""},
		{slash("2", " CORE-12 "), "/n-budget CORE-12", ""},
		{slash("5", ""), "/n-help", ""},
		{slash("6", "core-123 Code-Review"), "/n-geser<CORE-123>:code-review", ""},
		{slash("6", "CORE-123"), "", geserUsage},
		{slash("99", ""), "/n-help", ""},
		{&chat_app_dto.Message{Text: "/n-geser<CORE-1>:todo", ArgumentText: "/n-geser<CORE-1>:todo"}, "/n-geser<CORE-1>:todo", ""},
	}
	for _, c := range cases {
		got, hint := commandText(c.msg)
		if got != c.want || hint != c.hint {
			t.Errorf("commandText(%+v) = %q, %q; want %q, %q", c.msg, got, hint, c.want, c.hint)
		}
	}
}

func (f *fakeCommands) Geser(context.Context, string, string, string) chat_interactive_service.GeserResult {
	return chat_interactive_service.GeserResult{}
}

type fakeGeser struct {
	fakeCommands
	result chat_interactive_service.GeserResult
	got    string
}

func (f *fakeGeser) Geser(_ context.Context, email, key, target string) chat_interactive_service.GeserResult {
	f.got = email + "|" + key + "|" + target
	return f.result
}

func clickEvent(email string) chat_app_dto.Event {
	_, cards := chat_app_card.Build(google_chat_notifier_dto.ReminderMessage{IssueKey: "CORE-1", IssueURL: "u"})
	return chat_app_dto.Event{
		Type:    "CARD_CLICKED",
		Space:   chat_app_dto.Space{Name: "spaces/DM", Type: "DM"},
		User:    chat_app_dto.ChatUser{Name: "users/9", Email: email, DisplayName: "Dev"},
		Message: &chat_app_dto.Message{CardsV2: cards},
		Action: &chat_app_dto.Action{ActionMethodName: "geser", Parameters: []chat_app_dto.ActionParameter{
			{Key: "issueKey", Value: "CORE-1"}, {Key: "target", Value: "code-review"},
		}},
	}
}

func TestHandle_CardClickMovesAndUpdatesTheCard(t *testing.T) {
	g := &fakeGeser{result: chat_interactive_service.GeserResult{OK: true, StatusName: "Code Review"}}
	svc := New(Deps{Users: fakeUsers{dev: {Email: dev, FlagActive: true}}, States: &fakeStates{states: map[string]chat_app_dto.State{dev: {SpaceID: "spaces/DM"}}}, Commands: g})
	res := svc.Handle(context.Background(), clickEvent(dev))
	if g.got != dev+"|CORE-1|code-review" {
		t.Errorf("Geser call = %q", g.got)
	}
	if res.ActionResponse == nil || res.ActionResponse.Type != "UPDATE_MESSAGE" || len(res.CardsV2) != 1 {
		t.Fatalf("response = %+v", res)
	}
	secs := res.CardsV2[0].Card.Sections
	status := secs[len(secs)-1].Widgets[0].TextParagraph.Text
	if !strings.Contains(status, "Digeser ke Code Review oleh Dev") {
		t.Errorf("status line = %q", status)
	}
}

func TestHandle_CardClickFailureUpdatesWithReason(t *testing.T) {
	g := &fakeGeser{result: chat_interactive_service.GeserResult{Message: "Nggak ada jalur langsung"}}
	svc := New(Deps{Users: fakeUsers{dev: {Email: dev, FlagActive: true}}, States: &fakeStates{states: map[string]chat_app_dto.State{}}, Commands: g})
	res := svc.Handle(context.Background(), clickEvent(dev))
	secs := res.CardsV2[0].Card.Sections
	if !strings.Contains(secs[len(secs)-1].Widgets[0].TextParagraph.Text, "Tidak bisa digeser: Nggak ada jalur langsung") {
		t.Errorf("response = %+v", res)
	}
}

func TestHandle_CardClickByNonOwnerIsPrivateAndLeavesCard(t *testing.T) {
	g := &fakeGeser{result: chat_interactive_service.GeserResult{NotOwner: true, Message: "bukan tiket kamu"}}
	svc := New(Deps{Users: fakeUsers{dev: {Email: dev, FlagActive: true}}, States: &fakeStates{states: map[string]chat_app_dto.State{}}, Commands: g})
	res := svc.Handle(context.Background(), clickEvent(dev))
	if res.ActionResponse == nil || res.ActionResponse.Type != "NEW_MESSAGE" || res.PrivateMessageViewer == nil || res.Text != "bukan tiket kamu" || len(res.CardsV2) != 0 {
		t.Errorf("response = %+v", res)
	}
}
