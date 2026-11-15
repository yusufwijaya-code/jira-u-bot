package chat_log

import (
	"context"
	"errors"
	"testing"
	"time"

	"nudge/modules/chat_interactive/chat_interactive_service"
	"nudge/modules/chat_log/chat_log_dto"
)

type fakeRecorder struct{ entries []chat_log_dto.Entry }

func (f *fakeRecorder) Record(_ context.Context, e chat_log_dto.Entry) {
	f.entries = append(f.entries, e)
}

type fakeInteractive struct {
	chat_interactive_service.ChatInteractiveService
}

func (fakeInteractive) RouteCommand(_ context.Context, _, text string) string { return "ok " + text }

type fakeDM struct{ err error }

func (f fakeDM) SendPlainTextDM(context.Context, string, string) error { return f.err }

func TestLoggedCommands_RecordsTheCommandAndRoutesIt(t *testing.T) {
	rec := &fakeRecorder{}
	out := LoggedCommands{ChatInteractiveService: fakeInteractive{}, Log: rec}.RouteCommand(context.Background(), "dev@esb.co.id", "/n-team")
	if out != "ok /n-team" {
		t.Errorf("reply = %q", out)
	}
	if len(rec.entries) != 1 || rec.entries[0].Direction != "in" || rec.entries[0].Channel != "operator" || rec.entries[0].Body != "/n-team" {
		t.Errorf("entries = %+v", rec.entries)
	}
}

func TestLoggedReplies_RecordsOutcome(t *testing.T) {
	rec := &fakeRecorder{}
	_ = LoggedReplies{Inner: fakeDM{}, Log: rec}.SendPlainTextDM(context.Background(), "dev@esb.co.id", "🤖 hi")
	_ = LoggedReplies{Inner: fakeDM{err: errors.New("down")}, Log: rec}.SendPlainTextDM(context.Background(), "dev@esb.co.id", "🤖 hi")
	if len(rec.entries) != 2 || rec.entries[0].Status != "sent" || rec.entries[1].Status != "failed" || rec.entries[1].Error != "down" {
		t.Errorf("entries = %+v", rec.entries)
	}
}

type fakePurger struct {
	calls chan time.Time
}

func (f fakePurger) PurgeBefore(_ context.Context, t time.Time) (int64, error) {
	f.calls <- t
	return 0, nil
}

func TestRunRetention_PurgesOnStartAndStopsWithContext(t *testing.T) {
	p := fakePurger{calls: make(chan time.Time, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { RunRetention(ctx, p, 90*24*time.Hour); close(done) }()

	select {
	case cut := <-p.calls:
		if d := time.Since(cut); d < 89*24*time.Hour || d > 91*24*time.Hour {
			t.Errorf("cut-off %s ago", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no purge on start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunRetention did not stop")
	}
}
