package chat_delivery

import (
	"context"
	"errors"
	"testing"

	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/chat_log/chat_log_dto"
	"nudge/modules/google_chat_notifier/google_chat_notifier_dto"
)

type fakeRecorder struct{ entries []chat_log_dto.Entry }

func (f *fakeRecorder) Record(_ context.Context, e chat_log_dto.Entry) {
	f.entries = append(f.entries, e)
}

func loggedRouter(store *fakeStore, app *fakeApp, legacy *fakeLegacy, rec *fakeRecorder) *Router {
	d := Deps{Store: store, OperatorEmail: op, OperatorName: "Yusuf Wijaya", Log: rec}
	if app != nil {
		d.App = app
	}
	if legacy != nil {
		d.Legacy = legacy
	}
	return New(d)
}

func onlyEntry(t *testing.T, rec *fakeRecorder) chat_log_dto.Entry {
	t.Helper()
	if len(rec.entries) != 1 {
		t.Fatalf("recorded %d entries, want 1: %+v", len(rec.entries), rec.entries)
	}
	return rec.entries[0]
}

func TestLog_AppSendIsRecordedAsSentThroughTheApp(t *testing.T) {
	rec := &fakeRecorder{}
	store := &fakeStore{states: map[string]chat_app_dto.State{user: active(nil)}}
	if err := loggedRouter(store, &fakeApp{}, &fakeLegacy{}, rec).SendPlainTextDM(context.Background(), user, "halo"); err != nil {
		t.Fatal(err)
	}
	e := onlyEntry(t, rec)
	if e.Email != user || e.Direction != "out" || e.Kind != "message" || e.Channel != "app" || e.Status != "sent" || e.Body != "halo" {
		t.Errorf("entry = %+v", e)
	}
}

func TestLog_ReminderThroughOperatorIsKindReminder(t *testing.T) {
	rec := &fakeRecorder{}
	store := &fakeStore{states: map[string]chat_app_dto.State{user: active(ptr(chat_app_dto.SenderOperator))}}
	msg := google_chat_notifier_dto.ReminderMessage{AssigneeEmail: user, IssueKey: "CORE-1"}
	if err := loggedRouter(store, &fakeApp{}, &fakeLegacy{}, rec).SendReminder(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	e := onlyEntry(t, rec)
	if e.Kind != "reminder" || e.Channel != "operator" || e.Status != "sent" || e.Body == "" {
		t.Errorf("entry = %+v", e)
	}
}

func TestLog_FallbackIsRecordedAsFallback(t *testing.T) {
	rec := &fakeRecorder{}
	store := &fakeStore{states: map[string]chat_app_dto.State{user: active(nil)}}
	app := &fakeApp{err: errors.New("boom")}
	if err := loggedRouter(store, app, &fakeLegacy{}, rec).SendPlainTextDM(context.Background(), user, "x"); err != nil {
		t.Fatal(err)
	}
	if e := onlyEntry(t, rec); e.Channel != "fallback" || e.Status != "sent" {
		t.Errorf("entry = %+v", e)
	}
}

func TestLog_FailureAndSkipAreRecorded(t *testing.T) {
	rec := &fakeRecorder{}
	store := &fakeStore{states: map[string]chat_app_dto.State{op: {Email: op}}}
	// Operator account, not activated: skipped.
	_ = loggedRouter(store, &fakeApp{}, &fakeLegacy{}, rec).SendPlainTextDM(context.Background(), op, "x")
	if e := onlyEntry(t, rec); e.Status != "skipped" {
		t.Errorf("entry = %+v", e)
	}

	rec = &fakeRecorder{}
	store = &fakeStore{states: map[string]chat_app_dto.State{user: active(ptr(chat_app_dto.SenderOperator))}}
	legacy := &fakeLegacy{err: errors.New("oauth down")}
	if err := loggedRouter(store, &fakeApp{}, legacy, rec).SendPlainTextDM(context.Background(), user, "x"); err == nil {
		t.Fatal("expected an error")
	}
	if e := onlyEntry(t, rec); e.Status != "failed" || e.Channel != "operator" || e.Error != "oauth down" {
		t.Errorf("entry = %+v", e)
	}
}
