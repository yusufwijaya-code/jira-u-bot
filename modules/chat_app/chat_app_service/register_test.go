package chat_app_service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nudge/modules/chat_app/chat_app_card"
	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/registration/registration_service"
	"nudge/modules/user_setting/user_setting_dto"
)

// fakeRegistrar adds the user to users on success, like the real repository.
type fakeRegistrar struct {
	users      fakeUsers
	err        error
	calls      []string
	skippedDMs []bool
}

func (f *fakeRegistrar) Register(ctx context.Context, name, email string) error {
	f.calls = append(f.calls, name+"|"+email)
	if f.err != nil {
		return f.err
	}
	// The install reply is the welcome: the thank-you DM must be skipped.
	f.skippedDMs = append(f.skippedDMs, registration_service.ShouldSkipThankYouDM(ctx))
	f.users[email] = registration_dto.User{UserID: 42, Email: email, FlagActive: true}
	return nil
}

type fakeSwitch struct{ set []string }

func (f *fakeSwitch) SetNotificationsEnabled(_ context.Context, userID int64, enabled bool) (user_setting_dto.NotificationSwitch, error) {
	f.set = append(f.set, map[bool]string{true: "on", false: "off"}[enabled])
	return user_setting_dto.NotificationSwitch{Enabled: enabled}, nil
}

const newbie = "new@esb.co.id"

func registerSvc(reg *fakeRegistrar, states *fakeStates, sw *fakeSwitch, cmds *fakeCommands) Service {
	return New(Deps{Users: reg.users, States: states, Commands: cmds, Register: reg, Notifications: sw})
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

func TestAdded_UnknownUserIsRegisteredConnectedAndAskedAboutNotifications(t *testing.T) {
	reg := &fakeRegistrar{users: fakeUsers{}}
	states := &fakeStates{states: map[string]chat_app_dto.State{}}
	res := registerSvc(reg, states, &fakeSwitch{}, &fakeCommands{}).Handle(context.Background(), dmEvent("ADDED_TO_SPACE", newbie, nil))

	if len(reg.calls) != 1 || reg.calls[0] != "Dev|"+newbie {
		t.Fatalf("Register calls = %v, want display name + email", reg.calls)
	}
	if len(reg.skippedDMs) != 1 || !reg.skippedDMs[0] {
		t.Errorf("thank-you DM must be skipped, got %v", reg.skippedDMs)
	}
	if len(states.set) != 1 || states.set[0] != newbie+"|spaces/DM|users/9" {
		t.Errorf("SetSpace = %v, want the DM stored right away", states.set)
	}
	if !strings.HasPrefix(res.Text, "🎉 Selamat datang di Nudge") {
		t.Errorf("preview = %q", res.Text)
	}
	got := strings.Join(buttonFunctions(res), ",")
	if got != "notif_choice:1,notif_choice:0,nudge.cmd:" {
		t.Errorf("buttons = %s, want on / later / menu", got)
	}
}

func TestAdded_RegistrationFailuresExplainAndStoreNothing(t *testing.T) {
	cases := []struct {
		name, email, want string
		err               error
	}{
		{"no Jira account", newbie, "belum terdaftar sebagai akun Jira", registration_service.ErrJiraAccountNotFound},
		{"Jira down", newbie, "Gagal memverifikasi", errors.Join(registration_service.ErrJiraLookupUnavailable, errors.New("timeout"))},
		{"other domain", "someone@gmail.com", "hanya untuk akun @esb.co.id", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reg := &fakeRegistrar{users: fakeUsers{}, err: c.err}
			states := &fakeStates{states: map[string]chat_app_dto.State{}}
			res := registerSvc(reg, states, &fakeSwitch{}, &fakeCommands{}).Handle(context.Background(), dmEvent("ADDED_TO_SPACE", c.email, nil))
			if !strings.Contains(res.Text, c.want) {
				t.Errorf("reply = %q, want %q", res.Text, c.want)
			}
			if len(states.set) != 0 {
				t.Errorf("SetSpace must not run: %v", states.set)
			}
		})
	}
	// The domain is checked before Jira is ever asked.
	reg := &fakeRegistrar{users: fakeUsers{}}
	registerSvc(reg, &fakeStates{states: map[string]chat_app_dto.State{}}, &fakeSwitch{}, &fakeCommands{}).
		Handle(context.Background(), dmEvent("ADDED_TO_SPACE", "someone@gmail.com", nil))
	if len(reg.calls) != 0 {
		t.Errorf("Register called for another domain: %v", reg.calls)
	}
}

func TestAdded_DeactivatedUserIsNeverReRegistered(t *testing.T) {
	reg := &fakeRegistrar{users: fakeUsers{"off@esb.co.id": {Email: "off@esb.co.id", FlagActive: false}}}
	res := registerSvc(reg, &fakeStates{states: map[string]chat_app_dto.State{}}, &fakeSwitch{}, &fakeCommands{}).
		Handle(context.Background(), dmEvent("ADDED_TO_SPACE", "off@esb.co.id", nil))
	if !strings.Contains(res.Text, "dinonaktifkan") || len(reg.calls) != 0 {
		t.Errorf("reply = %q, Register calls = %v", res.Text, reg.calls)
	}
}

func TestAdded_RegisteredUserWithNotificationsOffIsAskedButNotReRegistered(t *testing.T) {
	reg := &fakeRegistrar{users: fakeUsers{dev: {Email: dev, FlagActive: true}}}
	res := registerSvc(reg, &fakeStates{states: map[string]chat_app_dto.State{}}, &fakeSwitch{}, &fakeCommands{}).
		Handle(context.Background(), dmEvent("ADDED_TO_SPACE", dev, nil))
	if len(reg.calls) != 0 {
		t.Errorf("Register must not run for a known user: %v", reg.calls)
	}
	if !strings.HasPrefix(res.Text, "✅ App Nudge aktif") || !strings.Contains(strings.Join(buttonFunctions(res), ","), "notif_choice:1") {
		t.Errorf("welcome = %q buttons %v", res.Text, buttonFunctions(res))
	}
}

func TestMessage_FirstMessageOfUnknownUserRegistersAndGreetsInsteadOfRunningIt(t *testing.T) {
	reg := &fakeRegistrar{users: fakeUsers{}}
	states := &fakeStates{states: map[string]chat_app_dto.State{}}
	cmds := &fakeCommands{}
	res := registerSvc(reg, states, &fakeSwitch{}, cmds).
		Handle(context.Background(), dmEvent("MESSAGE", newbie, &chat_app_dto.Message{Text: "/n-mytickets"}))
	if len(reg.calls) != 1 || len(states.set) != 1 || len(cmds.got) != 0 {
		t.Errorf("register=%v set=%v commands=%v", reg.calls, states.set, cmds.got)
	}
	if !strings.HasPrefix(res.Text, "🎉 Selamat datang") {
		t.Errorf("reply = %q", res.Text)
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
	reg := &fakeRegistrar{users: fakeUsers{dev: {UserID: 7, Email: dev, FlagActive: true}}}
	res := registerSvc(reg, &fakeStates{states: map[string]chat_app_dto.State{}}, sw, &fakeCommands{}).Handle(context.Background(), notifClick("1"))
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
	reg := &fakeRegistrar{users: fakeUsers{dev: {UserID: 7, Email: dev, FlagActive: true}}}
	res := registerSvc(reg, &fakeStates{states: map[string]chat_app_dto.State{}}, sw, &fakeCommands{}).Handle(context.Background(), notifClick("0"))
	if len(sw.set) != 0 {
		t.Errorf("'Nanti saja' must not save a choice: %v", sw.set)
	}
	if !strings.Contains(res.LogText, "masih mati") {
		t.Errorf("status = %q", res.LogText)
	}
}
