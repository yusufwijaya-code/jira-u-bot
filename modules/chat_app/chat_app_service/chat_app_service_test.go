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
	delay  time.Duration
	panics bool
	got    []string
}

func (f *fakeCommands) Command(_ context.Context, email, text string) chat_interactive_service.Reply {
	time.Sleep(f.delay)
	if f.panics {
		var m map[string]string
		m["nil deref"] = text // a real runtime panic, as from a nil map in a Jira path
	}
	f.got = append(f.got, email+"|"+text)
	return chat_interactive_service.Reply{Text: "reply to " + text}
}

func (f *fakeCommands) Action(_ context.Context, email, fn string, params map[string]string) chat_interactive_service.ActionReply {
	time.Sleep(f.delay)
	f.got = append(f.got, email+"|"+fn+"|"+params["key"]+"|"+params["from"])
	card := chat_app_dto.CardWithID{CardID: "c", Card: chat_app_dto.Card{Header: &chat_app_dto.CardHeader{Title: "after " + fn}}}
	return chat_interactive_service.ActionReply{
		Reply:  chat_interactive_service.Reply{Text: "did " + fn, Title: "after " + fn, Cards: []chat_app_dto.CardWithID{card}},
		Update: fn == "nudge.move",
	}
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
	// The first line is real text (Home / notification preview), plus a card.
	if !strings.HasPrefix(res.Text, "✅ App Nudge aktif") || len(res.CardsV2) != 1 || res.FallbackText != res.Text {
		t.Fatalf("welcome = %+v, want a preview line and a card", res)
	}
	var menu bool
	for _, w := range res.CardsV2[0].Card.Sections[0].Widgets {
		if w.ButtonList != nil && w.ButtonList.Buttons[0].OnClick.Action.Function == "nudge.cmd" {
			menu = true
		}
		if w.TextParagraph != nil && strings.Contains(w.TextParagraph.Text, "n-help") {
			t.Errorf("the /n-help hint must be a button, card text = %q", w.TextParagraph.Text)
		}
	}
	if !menu {
		t.Error("welcome card has no menu button")
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
		msg  *chat_app_dto.Message
		want string
	}{
		{slash("1", ""), "/n-mytickets"},
		{slash("1", " CORE-12 review "), "/n-mytickets CORE-12 review"},
		{slash("2", ""), "/n-dashboard"},
		{slash("3", "besok"), "/n-cuti besok"},
		{slash("4", "1 jam"), "/n-mute 1 jam"},
		{slash("5", ""), "/n-help"},
		{slash("6", ""), "/n-unmute"},
		{slash("99", ""), "/n-help"},
		{&chat_app_dto.Message{Text: "/n-geser<CORE-1>:todo", ArgumentText: "/n-geser<CORE-1>:todo"}, "/n-geser<CORE-1>:todo"},
	}
	for _, c := range cases {
		if got := commandText(c.msg); got != c.want {
			t.Errorf("commandText(%+v) = %q; want %q", c.msg, got, c.want)
		}
	}
}

func TestHandle_CommandCardIsSentAsCardWithFallbackAndLoggedAsText(t *testing.T) {
	rec := &fakeRecorder{}
	cmds := &cardCommands{}
	svc := New(Deps{Users: fakeUsers{dev: {Email: dev, FlagActive: true}}, States: &fakeStates{states: map[string]chat_app_dto.State{dev: {Email: dev, SpaceID: "spaces/DM"}}}, Commands: cmds, Log: rec})
	res := svc.Handle(context.Background(), dmEvent("MESSAGE", dev, &chat_app_dto.Message{Text: "/n-dashboard"}))
	// A card-only message reads "Sent an attachment" in Home: the title line
	// must go as text too.
	if len(res.CardsV2) != 1 || res.Text != "📊 Dashboard" || res.FallbackText != "📊 Dashboard" {
		t.Fatalf("response = %+v, want a card with its title line as text", res)
	}
	entries := rec.all()
	if out := entries[len(entries)-1]; out.Body != "*📊 Dashboard*\nisi" {
		t.Errorf("logged reply = %q, want the card's text rendering", out.Body)
	}
}

type cardCommands struct{ fakeCommands }

func (*cardCommands) Command(context.Context, string, string) chat_interactive_service.Reply {
	return chat_interactive_service.Reply{Text: "*📊 Dashboard*\nisi", Title: "📊 Dashboard", Cards: []chat_app_dto.CardWithID{{CardID: "d"}}}
}

func menuClick(fn string, params ...chat_app_dto.ActionParameter) chat_app_dto.Event {
	ev := dmEvent("CARD_CLICKED", dev, &chat_app_dto.Message{CardsV2: []chat_app_dto.CardWithID{{CardID: "old"}}})
	ev.Action = &chat_app_dto.Action{ActionMethodName: fn, Parameters: params}
	return ev
}

func TestHandle_MenuButtonThatUpdatesReplacesTheCard(t *testing.T) {
	cmds := &fakeCommands{}
	res := newSvc(&fakeStates{states: map[string]chat_app_dto.State{}}, cmds, nil).Handle(context.Background(),
		menuClick("nudge.move", chat_app_dto.ActionParameter{Key: "key", Value: "CORE-1"}))
	if res.ActionResponse == nil || res.ActionResponse.Type != chat_app_dto.ResponseUpdateMessage || len(res.CardsV2) != 1 {
		t.Fatalf("response = %+v, want UPDATE_MESSAGE with the new card", res)
	}
	if cmds.got[0] != dev+"|nudge.move|CORE-1|" {
		t.Errorf("Action call = %v", cmds.got)
	}
}

func TestHandle_MenuButtonOpeningAViewPostsANewMessage(t *testing.T) {
	res := newSvc(&fakeStates{states: map[string]chat_app_dto.State{}}, &fakeCommands{}, nil).Handle(context.Background(),
		menuClick("nudge.cmd", chat_app_dto.ActionParameter{Key: "cmd", Value: "/n-tiket"}))
	if res.ActionResponse == nil || res.ActionResponse.Type != chat_app_dto.ResponseNewMessage || len(res.CardsV2) != 1 {
		t.Fatalf("response = %+v, want NEW_MESSAGE with a card", res)
	}
}

func TestHandle_MenuButtonPassesFormInputs(t *testing.T) {
	cmds := &fakeCommands{}
	ev := menuClick("nudge.leave.add")
	ev.Common = &chat_app_dto.Common{FormInputs: map[string]chat_app_dto.FormInput{
		"from": {DateInput: &chat_app_dto.DateInput{MsSinceEpoch: "1795132800000"}}, // 2026-11-20 00:00 UTC
	}}
	newSvc(&fakeStates{states: map[string]chat_app_dto.State{}}, cmds, nil).Handle(context.Background(), ev)
	if len(cmds.got) != 1 || cmds.got[0] != dev+"|nudge.leave.add||2026-11-20" {
		t.Errorf("Action call = %v, want the picked date as from", cmds.got)
	}
}

func TestHandle_MenuButtonOfUnknownUserIsRefused(t *testing.T) {
	cmds := &fakeCommands{}
	ev := menuClick("nudge.cmd")
	ev.User.Email = "stranger@esb.co.id"
	res := newSvc(&fakeStates{states: map[string]chat_app_dto.State{}}, cmds, nil).Handle(context.Background(), ev)
	if len(cmds.got) != 0 || !strings.Contains(res.Text, "belum terdaftar") {
		t.Errorf("res=%+v calls=%v", res, cmds.got)
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

func TestHandle_PanickingCommandRepliesWithAnErrorInsteadOfCrashing(t *testing.T) {
	states := &fakeStates{states: map[string]chat_app_dto.State{dev: {Email: dev, SpaceID: "spaces/DM"}}}
	res := newSvc(states, &fakeCommands{panics: true}, nil).Handle(context.Background(), dmEvent("MESSAGE", dev, &chat_app_dto.Message{Text: "/n-team"}))
	if res.Text != errorText {
		t.Errorf("reply = %q, want %q", res.Text, errorText)
	}
}

func TestHandle_SlowPanickingCommandSendsTheErrorLater(t *testing.T) {
	states := &fakeStates{states: map[string]chat_app_dto.State{dev: {Email: dev, SpaceID: "spaces/DM"}}}
	app := &fakeApp{sent: make(chan chat_app_dto.OutgoingMessage, 1)}
	svc := New(Deps{
		Users: fakeUsers{dev: {Email: dev, FlagActive: true}}, States: states,
		Commands: &fakeCommands{delay: 50 * time.Millisecond, panics: true}, App: app, CommandTimeout: 10 * time.Millisecond,
	})
	if res := svc.Handle(context.Background(), dmEvent("MESSAGE", dev, &chat_app_dto.Message{Text: "/n-team"})); res.Text != processingText {
		t.Fatalf("immediate reply = %q", res.Text)
	}
	select {
	case msg := <-app.sent:
		if msg.Text != errorText {
			t.Errorf("late reply = %q, want %q", msg.Text, errorText)
		}
	case <-time.After(time.Second):
		t.Fatal("no late reply sent")
	}
}

type panickingApp struct{ called chan struct{} }

func (p *panickingApp) CreateMessage(context.Context, string, chat_app_dto.OutgoingMessage) error {
	close(p.called)
	panic("app client bug")
}

func TestHandle_PanicInTheLateReplyDoesNotCrash(t *testing.T) {
	states := &fakeStates{states: map[string]chat_app_dto.State{dev: {Email: dev, SpaceID: "spaces/DM"}}}
	app := &panickingApp{called: make(chan struct{})}
	svc := New(Deps{
		Users: fakeUsers{dev: {Email: dev, FlagActive: true}}, States: states,
		Commands: &fakeCommands{delay: 30 * time.Millisecond}, App: app, CommandTimeout: 5 * time.Millisecond,
	})
	svc.Handle(context.Background(), dmEvent("MESSAGE", dev, &chat_app_dto.Message{Text: "/n-team"}))
	select {
	case <-app.called:
		time.Sleep(20 * time.Millisecond) // the recover runs; a crash would end the test binary
	case <-time.After(time.Second):
		t.Fatal("late reply never attempted")
	}
}

type slowGeser struct {
	fakeCommands
	delay  time.Duration
	panics bool
	result chat_interactive_service.GeserResult
}

func (f *slowGeser) Geser(context.Context, string, string, string) chat_interactive_service.GeserResult {
	time.Sleep(f.delay)
	if f.panics {
		panic("geser bug")
	}
	return f.result
}

func TestHandle_SlowCardClickRepliesProcessingThenPostsTheOutcome(t *testing.T) {
	app := &fakeApp{sent: make(chan chat_app_dto.OutgoingMessage, 1)}
	g := &slowGeser{delay: 50 * time.Millisecond, result: chat_interactive_service.GeserResult{OK: true, StatusName: "Code Review"}}
	svc := New(Deps{
		Users: fakeUsers{dev: {Email: dev, FlagActive: true}}, States: &fakeStates{states: map[string]chat_app_dto.State{}},
		Commands: g, App: app, CommandTimeout: 10 * time.Millisecond,
	})
	res := svc.Handle(context.Background(), clickEvent(dev))
	if res.ActionResponse == nil || res.ActionResponse.Type != "NEW_MESSAGE" || res.Text != processingText || len(res.CardsV2) != 0 {
		t.Fatalf("immediate response = %+v", res)
	}
	select {
	case msg := <-app.sent:
		if !strings.Contains(msg.Text, "CORE-1") || !strings.Contains(msg.Text, "Digeser ke Code Review oleh Dev") {
			t.Errorf("late outcome = %q", msg.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("no late outcome sent")
	}
}

func TestHandle_PanickingCardClickUpdatesTheCardWithAnError(t *testing.T) {
	svc := New(Deps{
		Users: fakeUsers{dev: {Email: dev, FlagActive: true}}, States: &fakeStates{states: map[string]chat_app_dto.State{}},
		Commands: &slowGeser{panics: true},
	})
	res := svc.Handle(context.Background(), clickEvent(dev))
	if len(res.CardsV2) != 1 {
		t.Fatalf("response = %+v", res)
	}
	secs := res.CardsV2[0].Card.Sections
	if got := secs[len(secs)-1].Widgets[0].TextParagraph.Text; !strings.Contains(got, "Tidak bisa digeser") {
		t.Errorf("status line = %q", got)
	}
}
