package chat_app_service

import (
	"context"
	"strings"
	"testing"

	"nudge/modules/chat_app/chat_app_card"
	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/user_setting/user_setting_dto"
)

type fakeSwitch struct{ set []string }

func (f *fakeSwitch) SetNotificationsEnabled(_ context.Context, userID int64, enabled bool) (user_setting_dto.NotificationSwitch, error) {
	f.set = append(f.set, map[bool]string{true: "on", false: "off"}[enabled])
	return user_setting_dto.NotificationSwitch{Enabled: enabled}, nil
}

func notifSvc(users fakeUsers, states *fakeStates, sw *fakeSwitch, cmds *fakeCommands) Service {
	return New(Deps{Users: users, States: states, Commands: cmds, Notifications: sw})
}

func buttonFunctions(res chat_app_dto.Response) []string {
	var out []string
	for _, c := range res.CardsV2 {
		for _, s := range c.Card.Sections {
			for _, w := range s.Widgets {
				if w.ButtonList == nil {
					continue
				}
				for _, b := range w.ButtonList.Buttons {
					if b.OnClick.Action == nil {
						continue
					}
					on := ""
					for _, p := range b.OnClick.Action.Parameters {
						if p.Key == "on" {
							on = p.Value
						}
					}
					out = append(out, b.OnClick.Action.Function+":"+on)
				}
			}
		}
	}
	return out
}

func TestUnregistered_InstallOrMessageIsToldToRegisterOnTheWeb(t *testing.T) {
	states := &fakeStates{states: map[string]chat_app_dto.State{}}
	cmds := &fakeCommands{}
	svc := notifSvc(fakeUsers{}, states, &fakeSwitch{}, cmds)
	for _, ev := range []chat_app_dto.Event{
		dmEvent("ADDED_TO_SPACE", "new@esb.co.id", nil),
		dmEvent("MESSAGE", "new@esb.co.id", &chat_app_dto.Message{Text: "/n-mytickets"}),
	} {
		res := svc.Handle(context.Background(), ev)
		if !strings.Contains(res.Text, "belum terdaftar") || !strings.Contains(res.Text, "https://nudge.labcup.tech") {
			t.Errorf("%s reply = %q, want the register-on-web message", ev.Type, res.Text)
		}
	}
	if len(states.set) != 0 || len(cmds.got) != 0 {
		t.Errorf("nothing may be stored or run for an unknown sender: set=%v commands=%v", states.set, cmds.got)
	}
}

func TestAdded_InstallingTheAppTurnsNotificationsOn(t *testing.T) {
	sw := &fakeSwitch{}
	res := notifSvc(fakeUsers{dev: {UserID: 7, Email: dev, FlagActive: true}}, &fakeStates{states: map[string]chat_app_dto.State{}}, sw, &fakeCommands{}).
		Handle(context.Background(), dmEvent("ADDED_TO_SPACE", dev, nil))
	if strings.Join(sw.set, ",") != "on" {
		t.Errorf("switch = %v, want on", sw.set)
	}
	if !strings.HasPrefix(res.Text, "✅ App Nudge aktif") {
		t.Errorf("preview = %q", res.Text)
	}
	if got := strings.Join(buttonFunctions(res), ","); got != "nudge.cmd:" {
		t.Errorf("buttons = %s, want only the menu (notifications are already on)", got)
	}
}

type failingSwitch struct{}

func (failingSwitch) SetNotificationsEnabled(context.Context, int64, bool) (user_setting_dto.NotificationSwitch, error) {
	return user_setting_dto.NotificationSwitch{}, context.DeadlineExceeded
}

func TestAdded_SwitchFailureStillOffersTheButtons(t *testing.T) {
	res := New(Deps{Users: fakeUsers{dev: {UserID: 7, Email: dev, FlagActive: true}}, States: &fakeStates{states: map[string]chat_app_dto.State{}}, Commands: &fakeCommands{}, Notifications: failingSwitch{}}).
		Handle(context.Background(), dmEvent("ADDED_TO_SPACE", dev, nil))
	if got := strings.Join(buttonFunctions(res), ","); got != "notif_choice:1,notif_choice:0,nudge.cmd:" {
		t.Errorf("buttons = %s, want on / later / menu", got)
	}
}

func TestAdded_NotificationsAlreadyOnShowsOnlyTheMenu(t *testing.T) {
	res := notifSvc(fakeUsers{dev: {Email: dev, FlagActive: true, NotificationsEnabled: true}}, &fakeStates{states: map[string]chat_app_dto.State{}}, &fakeSwitch{}, &fakeCommands{}).
		Handle(context.Background(), dmEvent("ADDED_TO_SPACE", dev, nil))
	if got := strings.Join(buttonFunctions(res), ","); got != "nudge.cmd:" {
		t.Errorf("buttons = %s, want only the menu", got)
	}
}

func notifClick(on string) chat_app_dto.Event {
	ev := dmEvent("CARD_CLICKED", dev, nil)
	_, cards := chat_app_card.WelcomeCard("✅ hi\n\nbody\n\n💡 Ketik */n-help* buat lihat command", true)
	ev.Message = &chat_app_dto.Message{CardsV2: cards}
	ev.Action = &chat_app_dto.Action{ActionMethodName: chat_app_card.NotifChoiceFunction, Parameters: []chat_app_dto.ActionParameter{{Key: "on", Value: on}}}
	return ev
}

func TestNotifChoice_OnTurnsTheSwitchOnAndKeepsTheMenu(t *testing.T) {
	sw := &fakeSwitch{}
	res := notifSvc(fakeUsers{dev: {UserID: 7, Email: dev, FlagActive: true}}, &fakeStates{states: map[string]chat_app_dto.State{}}, sw, &fakeCommands{}).
		Handle(context.Background(), notifClick("1"))
	if strings.Join(sw.set, ",") != "on" {
		t.Errorf("switch = %v, want on", sw.set)
	}
	if res.ActionResponse == nil || res.ActionResponse.Type != chat_app_dto.ResponseUpdateMessage {
		t.Fatalf("response = %+v, want the card updated", res)
	}
	if got := strings.Join(buttonFunctions(res), ","); got != "nudge.cmd:" {
		t.Errorf("buttons after choice = %s, want only the menu", got)
	}
	if !strings.Contains(res.LogText, "Notifikasi dinyalakan") {
		t.Errorf("status = %q", res.LogText)
	}
}

func TestNotifChoice_LaterRecordsNothing(t *testing.T) {
	sw := &fakeSwitch{}
	res := notifSvc(fakeUsers{dev: {UserID: 7, Email: dev, FlagActive: true}}, &fakeStates{states: map[string]chat_app_dto.State{}}, sw, &fakeCommands{}).
		Handle(context.Background(), notifClick("0"))
	if len(sw.set) != 0 {
		t.Errorf("'Nanti saja' must not save a choice: %v", sw.set)
	}
	if !strings.Contains(res.LogText, "masih mati") {
		t.Errorf("status = %q", res.LogText)
	}
}

func TestNotifChoice_UnknownUserIsRefusedPrivately(t *testing.T) {
	sw := &fakeSwitch{}
	ev := notifClick("1")
	ev.User.Email = "new@esb.co.id"
	res := notifSvc(fakeUsers{}, &fakeStates{states: map[string]chat_app_dto.State{}}, sw, &fakeCommands{}).Handle(context.Background(), ev)
	if len(sw.set) != 0 || res.PrivateMessageViewer == nil {
		t.Errorf("switch=%v response=%+v, want a private refusal", sw.set, res)
	}
}
