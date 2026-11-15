package chat_delivery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"nudge/base/helpers/curl_helper"
	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/google_chat_notifier/google_chat_notifier_dto"
)

type fakeStore struct {
	states  map[string]chat_app_dto.State
	cleared []string
}

func (f *fakeStore) FindByEmail(_ context.Context, email string) (chat_app_dto.State, bool, error) {
	s, ok := f.states[email]
	if !ok {
		return chat_app_dto.State{Email: email}, false, nil
	}
	return s, true, nil
}

func (f *fakeStore) ClearByEmail(_ context.Context, email string) error {
	f.cleared = append(f.cleared, email)
	return nil
}

type fakeApp struct {
	err  error
	sent []chat_app_dto.OutgoingMessage
	to   []string
}

func (f *fakeApp) CreateMessage(_ context.Context, space string, msg chat_app_dto.OutgoingMessage) error {
	f.to = append(f.to, space)
	f.sent = append(f.sent, msg)
	return f.err
}

type fakeLegacy struct {
	err  error
	sent map[string]string
}

func (f *fakeLegacy) SendPlainTextDM(_ context.Context, email, text string) error {
	if f.sent == nil {
		f.sent = map[string]string{}
	}
	f.sent[email] = text
	return f.err
}

func ptr(s string) *string { return &s }

const (
	op   = "bot@esb.co.id"
	user = "dev@esb.co.id"
)

func active(sender *string) chat_app_dto.State {
	return chat_app_dto.State{Email: user, SpaceID: "spaces/U", Sender: sender}
}

func newRouter(store *fakeStore, app *fakeApp, legacy *fakeLegacy) *Router {
	d := Deps{Store: store, OperatorEmail: op, OperatorName: "Yusuf Wijaya"}
	if app != nil {
		d.App = app
	}
	if legacy != nil {
		d.Legacy = legacy
	}
	return New(d)
}

func TestEffectiveSender(t *testing.T) {
	cases := []struct {
		name      string
		s         chat_app_dto.State
		isOp, app bool
		want      string
	}{
		{"app disabled", active(ptr("nudge")), false, false, "operator"},
		{"operator account", chat_app_dto.State{}, true, true, "nudge"},
		{"explicit operator", active(ptr("operator")), false, true, "operator"},
		{"explicit nudge", chat_app_dto.State{Sender: ptr("nudge")}, false, true, "nudge"},
		{"auto activated", active(nil), false, true, "nudge"},
		{"auto not activated", chat_app_dto.State{}, false, true, "operator"},
		{"garbage value", active(ptr("x")), false, true, "nudge"},
	}
	for _, c := range cases {
		if got := EffectiveSender(c.s, c.isOp, c.app); got != c.want {
			t.Errorf("%s: EffectiveSender = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSendPlainTextDM_ActivatedGoesThroughApp(t *testing.T) {
	store := &fakeStore{states: map[string]chat_app_dto.State{user: active(nil)}}
	app, legacy := &fakeApp{}, &fakeLegacy{}
	if err := newRouter(store, app, legacy).SendPlainTextDM(context.Background(), user, "halo"); err != nil {
		t.Fatal(err)
	}
	if len(app.sent) != 1 || app.to[0] != "spaces/U" || app.sent[0].Text != "halo" || len(legacy.sent) != 0 {
		t.Errorf("app=%+v legacy=%+v", app.sent, legacy.sent)
	}
}

func TestSendPlainTextDM_NotActivatedAutoUsesOperatorWithInvite(t *testing.T) {
	store := &fakeStore{states: map[string]chat_app_dto.State{}}
	app, legacy := &fakeApp{}, &fakeLegacy{}
	if err := newRouter(store, app, legacy).SendPlainTextDM(context.Background(), user, "halo"); err != nil {
		t.Fatal(err)
	}
	if len(app.sent) != 0 || !strings.HasPrefix(legacy.sent[user], "halo") || !strings.Contains(legacy.sent[user], "aktifkan di https://nudge.labcup.tech") {
		t.Errorf("legacy text = %q", legacy.sent[user])
	}
}

func TestSendPlainTextDM_ExplicitOperatorHasNoFooter(t *testing.T) {
	store := &fakeStore{states: map[string]chat_app_dto.State{user: active(ptr("operator"))}}
	app, legacy := &fakeApp{}, &fakeLegacy{}
	_ = newRouter(store, app, legacy).SendPlainTextDM(context.Background(), user, "halo")
	if legacy.sent[user] != "halo" || len(app.sent) != 0 {
		t.Errorf("legacy=%q app=%d", legacy.sent[user], len(app.sent))
	}
}

func TestSendPlainTextDM_AppErrorFallsBackToOperator(t *testing.T) {
	store := &fakeStore{states: map[string]chat_app_dto.State{user: active(ptr("nudge"))}}
	app, legacy := &fakeApp{err: fmt.Errorf("wrap: %w", &curl_helper.HTTPError{StatusCode: 500})}, &fakeLegacy{}
	if err := newRouter(store, app, legacy).SendPlainTextDM(context.Background(), user, "halo"); err != nil {
		t.Fatalf("fallback succeeded, want nil error, got %v", err)
	}
	if !strings.Contains(legacy.sent[user], "dikirim lewat akun Yusuf Wijaya karena app Nudge sedang gangguan") {
		t.Errorf("legacy text = %q", legacy.sent[user])
	}
	if len(store.cleared) != 0 {
		t.Errorf("a 500 must not forget the space: cleared=%v", store.cleared)
	}
}

func TestSendPlainTextDM_AppGoneClearsStateAndFallsBack(t *testing.T) {
	store := &fakeStore{states: map[string]chat_app_dto.State{user: active(ptr("nudge"))}}
	app, legacy := &fakeApp{err: &curl_helper.HTTPError{StatusCode: 404}}, &fakeLegacy{}
	if err := newRouter(store, app, legacy).SendPlainTextDM(context.Background(), user, "halo"); err != nil {
		t.Fatal(err)
	}
	if len(store.cleared) != 1 || legacy.sent[user] == "" {
		t.Errorf("cleared=%v legacy=%q", store.cleared, legacy.sent[user])
	}
}

func TestOperatorAccount_NeverUsesOperatorDM(t *testing.T) {
	store := &fakeStore{states: map[string]chat_app_dto.State{}}
	app, legacy := &fakeApp{}, &fakeLegacy{}
	err := newRouter(store, app, legacy).SendPlainTextDM(context.Background(), "BOT@esb.co.id", "halo")
	if !errors.Is(err, ErrRecipientNotActivated) || len(legacy.sent) != 0 {
		t.Errorf("err=%v legacy=%v", err, legacy.sent)
	}

	store.states["BOT@esb.co.id"] = chat_app_dto.State{SpaceID: "spaces/OP"}
	app.err = errors.New("boom")
	err = newRouter(store, app, legacy).SendPlainTextDM(context.Background(), "BOT@esb.co.id", "halo")
	if err == nil || errors.Is(err, ErrRecipientNotActivated) || len(legacy.sent) != 0 {
		t.Errorf("operator app failure: err=%v legacy=%v", err, legacy.sent)
	}
}

func TestAppDisabled_StoredNudgeChoiceUsesOperatorWithoutInvite(t *testing.T) {
	store := &fakeStore{states: map[string]chat_app_dto.State{user: {Email: user, Sender: ptr("nudge")}}}
	legacy := &fakeLegacy{}
	if err := newRouter(store, nil, legacy).SendPlainTextDM(context.Background(), user, "halo"); err != nil {
		t.Fatal(err)
	}
	if legacy.sent[user] != "halo" {
		t.Errorf("legacy text = %q", legacy.sent[user])
	}
}

func TestNoLegacy_NotActivatedIsSkipped(t *testing.T) {
	store := &fakeStore{states: map[string]chat_app_dto.State{}}
	err := newRouter(store, &fakeApp{}, nil).SendPlainTextDM(context.Background(), user, "halo")
	if !errors.Is(err, ErrRecipientNotActivated) {
		t.Errorf("err = %v, want ErrRecipientNotActivated", err)
	}
}

func TestBothFail_ReturnsRealError(t *testing.T) {
	store := &fakeStore{states: map[string]chat_app_dto.State{user: {Email: user, Sender: ptr("nudge")}}}
	legacy := &fakeLegacy{err: errors.New("oauth down")}
	err := newRouter(store, &fakeApp{}, legacy).SendPlainTextDM(context.Background(), user, "halo")
	if err == nil || errors.Is(err, ErrRecipientNotActivated) {
		t.Errorf("err = %v, want the operator DM error", err)
	}
}

func TestSendReminder_UsesCardsOnAppAndTextOnOperator(t *testing.T) {
	msg := google_chat_notifier_dto.ReminderMessage{Text: "full text", AssigneeEmail: user, IssueKey: "CORE-1"}
	store := &fakeStore{states: map[string]chat_app_dto.State{user: active(nil)}}
	app, legacy := &fakeApp{}, &fakeLegacy{}
	r := newRouter(store, app, legacy)
	r.d.Cards = func(m google_chat_notifier_dto.ReminderMessage) (string, []chat_app_dto.CardWithID) {
		return "short", []chat_app_dto.CardWithID{{CardID: m.IssueKey}}
	}
	if err := r.SendReminder(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if app.sent[0].Text != "short" || len(app.sent[0].CardsV2) != 1 {
		t.Errorf("app message = %+v", app.sent[0])
	}

	app.err = errors.New("down")
	if err := r.SendReminder(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(legacy.sent[user], "full text") {
		t.Errorf("fallback text = %q, want the full plain text", legacy.sent[user])
	}
}

func TestOperatorAccount_AppGoneClearsStateAndReturnsTheAppError(t *testing.T) {
	for _, code := range []int{403, 404} {
		store := &fakeStore{states: map[string]chat_app_dto.State{op: {Email: op, SpaceID: "spaces/OP"}}}
		gone := &curl_helper.HTTPError{StatusCode: code}
		legacy := &fakeLegacy{}
		err := newRouter(store, &fakeApp{err: gone}, legacy).SendPlainTextDM(context.Background(), op, "halo")
		if !errors.Is(err, gone) || errors.Is(err, ErrRecipientNotActivated) {
			t.Errorf("%d: err = %v, want the app error", code, err)
		}
		if len(store.cleared) != 1 || store.cleared[0] != op {
			t.Errorf("%d: cleared = %v, want the operator's state reset (AC1.3)", code, store.cleared)
		}
		if len(legacy.sent) != 0 {
			t.Errorf("%d: the operator account must never fall back to its own DM: %v", code, legacy.sent)
		}
	}
}

func TestNoOperatorOAuth_AppFailureReturnsTheAppError(t *testing.T) {
	store := &fakeStore{states: map[string]chat_app_dto.State{user: active(ptr("nudge"))}}
	appErr := &curl_helper.HTTPError{StatusCode: 500}
	err := newRouter(store, &fakeApp{err: appErr}, nil).SendPlainTextDM(context.Background(), user, "halo")
	if !errors.Is(err, appErr) {
		t.Errorf("err = %v, want the app error (no fallback without operator OAuth)", err)
	}
}

func TestNoOperatorOAuth_ExplicitOperatorIsSkipped(t *testing.T) {
	store := &fakeStore{states: map[string]chat_app_dto.State{user: active(ptr("operator"))}}
	app := &fakeApp{}
	err := newRouter(store, app, nil).SendPlainTextDM(context.Background(), user, "halo")
	if !errors.Is(err, ErrRecipientNotActivated) || len(app.sent) != 0 {
		t.Errorf("err = %v app=%d, want ErrRecipientNotActivated (AC1.4a)", err, len(app.sent))
	}
}

type failingStore struct{ fakeStore }

func (failingStore) FindByEmail(context.Context, string) (chat_app_dto.State, bool, error) {
	return chat_app_dto.State{}, false, errors.New("db down")
}

func TestStateLookupErrorIsReturnedAndNothingIsSent(t *testing.T) {
	app, legacy := &fakeApp{}, &fakeLegacy{}
	r := New(Deps{Store: &failingStore{}, App: app, Legacy: legacy, OperatorEmail: op})
	err := r.SendPlainTextDM(context.Background(), user, "halo")
	if err == nil || errors.Is(err, ErrRecipientNotActivated) || len(app.sent) != 0 || len(legacy.sent) != 0 {
		t.Errorf("err=%v app=%d legacy=%d", err, len(app.sent), len(legacy.sent))
	}
}
