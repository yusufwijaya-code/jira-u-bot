package status_watch_service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/google_chat_notifier/google_chat_notifier_dto"
	"nudge/modules/issue_session/issue_session_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/status_watch/status_watch_dto"
	"nudge/modules/user_setting/user_setting_dto"
)

// --- fakes ---

type fakeJira struct {
	issues     []jira_client_dto.Issue
	err        error
	windows    []time.Duration
	changelogs map[string][]jira_client_dto.ChangelogEntry
}

func (f *fakeJira) SearchSprintIssuesUpdatedWithin(_ context.Context, _ []string, within time.Duration) ([]jira_client_dto.Issue, error) {
	f.windows = append(f.windows, within)
	return f.issues, f.err
}

func (f *fakeJira) GetRecentChangelog(_ context.Context, key string) ([]jira_client_dto.ChangelogEntry, error) {
	return f.changelogs[key], nil
}

type fakeUsers struct{ users []registration_dto.User }

func (f fakeUsers) FindAll(context.Context) ([]registration_dto.User, error) { return f.users, nil }

type fakeSettings struct{ s user_setting_dto.UserSetting }

func (f fakeSettings) GetEffectiveForEmail(context.Context, string, int64) user_setting_dto.UserSetting {
	return f.s
}

type fakeSessions map[string]*issue_session_dto.IssueSession

func (f fakeSessions) GetSessionByIssueKey(_ context.Context, key string) (*issue_session_dto.IssueSession, bool, error) {
	s, ok := f[key]
	return s, ok, nil
}

type fakeSnapshots struct {
	rows map[string]status_watch_dto.Snapshot
}

func (f *fakeSnapshots) Find(_ context.Context, keys []string) (map[string]status_watch_dto.Snapshot, error) {
	out := map[string]status_watch_dto.Snapshot{}
	for _, k := range keys {
		if s, ok := f.rows[k]; ok {
			out[k] = s
		}
	}
	return out, nil
}

func (f *fakeSnapshots) Upsert(_ context.Context, s status_watch_dto.Snapshot) error {
	f.rows[s.IssueKey] = s
	return nil
}

func (f *fakeSnapshots) DeleteNotSeenSince(context.Context, time.Time) (int64, error) { return 0, nil }

type fakeNotifier struct {
	sent []google_chat_notifier_dto.ReminderMessage
}

func (f *fakeNotifier) SendReminder(_ context.Context, m google_chat_notifier_dto.ReminderMessage) error {
	f.sent = append(f.sent, m)
	return nil
}

// --- fixture ---

var fixedNow = time.Date(2026, 11, 16, 10, 20, 0, 0, time.FixedZone("WIB", 7*3600)) // Monday

const activeSprint = `[{"id": 7988, "name": "S", "state": "active", "boardId": 26}]`

func ticket(key, status, account string) jira_client_dto.Issue {
	return jira_client_dto.Issue{Key: key, Fields: jira_client_dto.IssueFields{
		Summary: key + " summary", Status: jira_client_dto.Status{Name: status},
		Assignee: &jira_client_dto.Assignee{AccountID: account}, SprintRaw: json.RawMessage(activeSprint),
	}}
}

func moved(at, from, to, who string) jira_client_dto.ChangelogEntry {
	return jira_client_dto.ChangelogEntry{
		Created: at, Author: &jira_client_dto.ChangelogAuthor{DisplayName: who},
		Items: []jira_client_dto.ChangelogEntryItem{{Field: "status", FromString: from, ToString: to}},
	}
}

type fixture struct {
	svc      *statusWatchServiceImpl
	jira     *fakeJira
	snaps    *fakeSnapshots
	dm       *fakeNotifier
	settings *fakeSettings
	calendar business_hours_dto.HolidayCalendar
	sessions fakeSessions
}

func newFixture() *fixture {
	f := &fixture{
		jira:     &fakeJira{changelogs: map[string][]jira_client_dto.ChangelogEntry{}},
		snaps:    &fakeSnapshots{rows: map[string]status_watch_dto.Snapshot{}},
		dm:       &fakeNotifier{},
		settings: &fakeSettings{s: user_setting_dto.Default()},
		sessions: fakeSessions{},
	}
	return f
}

func (f *fixture) build() *statusWatchServiceImpl {
	svc := NewStatusWatchService(Deps{
		Jira: f.jira,
		Users: fakeUsers{users: []registration_dto.User{
			{Email: "dev@esb.co.id", JiraAccountID: "acc-dev", FlagActive: true},
			{Email: "bot@esb.co.id", JiraAccountID: "acc-bot", FlagActive: true},
		}},
		Settings: settingsRef{f.settings}, Sessions: f.sessions, Snapshots: f.snaps,
		BusinessHours: business_hours_service.NewBusinessHoursService(), Calendar: f.calendar,
		Notifier: f.dm, JiraBaseURL: "https://jira.example/",
	}).(*statusWatchServiceImpl)
	svc.now = func() time.Time { return fixedNow }
	f.svc = svc
	return svc
}

// settingsRef reads the fixture's settings at call time.
type settingsRef struct{ f *fakeSettings }

func (r settingsRef) GetEffectiveForEmail(ctx context.Context, email string, board int64) user_setting_dto.UserSetting {
	return r.f.GetEffectiveForEmail(ctx, email, board)
}

// cycle runs two cycles: the first seeds `from`, the second sees `to`.
func (f *fixture) cycle(t *testing.T, key, from, to, account string) {
	t.Helper()
	f.jira.issues = []jira_client_dto.Issue{ticket(key, from, account)}
	if err := f.svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.jira.issues = []jira_client_dto.Issue{ticket(key, to, account)}
	if err := f.svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// --- tests ---

func TestRunOnce_FirstSightingIsSilentThenAChangeIsSent(t *testing.T) {
	f := newFixture()
	f.jira.changelogs["OMS-1"] = []jira_client_dto.ChangelogEntry{
		moved("2026-11-16T09:00:00.000+0700", "Backlog", "To Do", "PO"),
		moved("2026-11-16T10:15:00.000+0700", "To Do", "Code Process", "Anwar"),
	}
	f.build()

	f.jira.issues = []jira_client_dto.Issue{ticket("OMS-1", "To Do", "acc-dev")}
	if err := f.svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.dm.sent) != 0 || f.snaps.rows["OMS-1"].Status != "To Do" {
		t.Fatalf("first sighting: sent %d, snapshot %+v — want silent + recorded", len(f.dm.sent), f.snaps.rows["OMS-1"])
	}
	if f.jira.windows[0] != 0 {
		t.Error("the first cycle must read everything (no updated filter)")
	}

	f.jira.issues = []jira_client_dto.Issue{ticket("OMS-1", "Code Process", "acc-dev")}
	if err := f.svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.dm.sent) != 1 {
		t.Fatalf("sent %d, want 1", len(f.dm.sent))
	}
	text := f.dm.sent[0].Text
	for _, want := range []string{"*OMS-1* — OMS-1 summary", "*To Do* → *Code Process*", "oleh Anwar", "10:15 WIB", "https://jira.example/browse/OMS-1"} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q lacks %q", text, want)
		}
	}
	if w := f.jira.windows[1]; w != time.Minute {
		t.Errorf("second cycle window = %v, want time since the last success + 1 minute", w)
	}

	// Same status again: nothing new.
	if err := f.svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.dm.sent) != 1 {
		t.Errorf("an unchanged status was sent again (%d)", len(f.dm.sent))
	}
}

func TestRunOnce_SkipsWhenTheUserDoesNotWantIt(t *testing.T) {
	muted := fixedNow.Add(time.Hour)
	cases := map[string]func(f *fixture){
		"master switch off": func(f *fixture) { f.settings.s.NotificationsOff = true },
		"toggle off":        func(f *fixture) { f.settings.s.NotifyStatusChange = false },
		"status not picked": func(f *fixture) { f.settings.s.StatusChangeStatuses = []string{"Done"} },
		"muted":             func(f *fixture) { f.settings.s.MutedUntil = &muted },
		"on leave": func(f *fixture) {
			f.calendar = business_hours_dto.HolidayCalendar{Holidays: map[string]bool{"2026-11-16": true}}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			setup(f)
			f.build()
			f.cycle(t, "OMS-1", "To Do", "Code Process", "acc-dev")
			if len(f.dm.sent) != 0 {
				t.Errorf("sent %d, want 0", len(f.dm.sent))
			}
			if f.snaps.rows["OMS-1"].Status != "Code Process" {
				t.Error("the snapshot must still move on (no late notice)")
			}
		})
	}
}

func TestRunOnce_PickedStatusIsSent(t *testing.T) {
	f := newFixture()
	f.settings.s.StatusChangeStatuses = []string{"code process"}
	f.build()
	f.cycle(t, "OMS-1", "To Do", "Code Process", "acc-dev")
	if len(f.dm.sent) != 1 {
		t.Fatalf("sent %d, want 1", len(f.dm.sent))
	}
}

func TestRunOnce_LeavingTrackedStatusAddsTheBudgetLine(t *testing.T) {
	cases := []struct {
		name, key, from, to string
		sp                  float64
		used                int64
		want                string
	}{
		{"finished early", "CORE-1", "CODE REVIEW", "Code Completed", 2, 3600, "⏱ Selesai 0 hari 1 jam 0 menit 0 detik lebih awal dari budget 0 hari 2 jam 0 menit 0 detik"},
		{"back to To Do", "OMS-2", "Code Process", "To Do", 2, 3600, "⏱ Waktu terpakai 0 hari 1 jam 0 menit 0 detik dari budget 0 hari 2 jam 0 menit 0 detik"},
		{"over budget", "OMS-3", "Code Process", "Code Review", 1, 7200, "⏱ Waktu terpakai 0 hari 2 jam 0 menit 0 detik dari budget 0 hari 1 jam 0 menit 0 detik (lewat 0 hari 1 jam 0 menit 0 detik)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture()
			f.sessions[c.key] = &issue_session_dto.IssueSession{IssueKey: c.key, StoryPoints: c.sp, AccumulatedSeconds: c.used}
			f.build()
			f.cycle(t, c.key, c.from, c.to, "acc-dev")
			if len(f.dm.sent) != 1 || !strings.Contains(f.dm.sent[0].Text, c.want) {
				t.Fatalf("sent %v, want a line %q", f.dm.sent, c.want)
			}
		})
	}
}

func TestRunOnce_MovesWithinTrackedStatusesHaveNoBudgetLine(t *testing.T) {
	f := newFixture()
	f.sessions["CORE-1"] = &issue_session_dto.IssueSession{IssueKey: "CORE-1", StoryPoints: 2, AccumulatedSeconds: 3600}
	f.build()
	f.cycle(t, "CORE-1", "Code Process", "CODE REVIEW", "acc-dev")
	if len(f.dm.sent) != 1 || strings.Contains(f.dm.sent[0].Text, "⏱") {
		t.Fatalf("sent %v, want one notice without ⏱", f.dm.sent)
	}
}

func TestRunOnce_TheBotAccountGoesThroughTheSameNotifier(t *testing.T) {
	f := newFixture()
	f.build()
	f.cycle(t, "OMS-1", "To Do", "Code Process", "acc-bot")
	if len(f.dm.sent) != 1 || f.dm.sent[0].AssigneeEmail != "bot@esb.co.id" {
		t.Fatalf("sent %+v, want one notice to the bot account (no webhook, AC1.6)", f.dm.sent)
	}
}

func TestRunOnce_UnregisteredAssigneesAreIgnored(t *testing.T) {
	f := newFixture()
	f.build()
	f.cycle(t, "OMS-9", "To Do", "Code Process", "acc-stranger")
	if len(f.dm.sent) != 0 || len(f.snaps.rows) != 0 {
		t.Fatalf("stranger: sent %d, snapshots %v", len(f.dm.sent), f.snaps.rows)
	}
}

func TestRunOnce_HandOffNotifiesTheNewAndThePreviousAssignee(t *testing.T) {
	f := newFixture()
	f.sessions["OMS-1"] = &issue_session_dto.IssueSession{IssueKey: "OMS-1", StoryPoints: 2, AccumulatedSeconds: 3600, AssigneeEmail: "bot@esb.co.id"}
	f.build()
	// The bot account worked on it, then moved it to Code Review and assigned dev.
	f.snaps.rows["OMS-1"] = status_watch_dto.Snapshot{IssueKey: "OMS-1", Status: "Code Process", AssigneeEmail: "bot@esb.co.id"}
	f.jira.issues = []jira_client_dto.Issue{ticket("OMS-1", "Code Review", "acc-dev")}
	if err := f.svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.dm.sent) != 2 {
		t.Fatalf("sent %d, want 2 (new + previous assignee)", len(f.dm.sent))
	}
	var previous *google_chat_notifier_dto.ReminderMessage
	for i := range f.dm.sent {
		if f.dm.sent[i].AssigneeEmail == "bot@esb.co.id" {
			previous = &f.dm.sent[i]
		}
	}
	if previous == nil || !strings.Contains(previous.Text, "lebih awal") {
		t.Errorf("the previous assignee's notice is missing or lacks the budget line: %+v", previous)
	}
	if f.snaps.rows["OMS-1"].AssigneeEmail != "dev@esb.co.id" {
		t.Error("the snapshot must follow the new assignee")
	}
}

func TestRunOnce_ReadsEverythingAgainEveryHour(t *testing.T) {
	f := newFixture()
	f.build()
	now := fixedNow
	f.svc.now = func() time.Time { return now }
	f.jira.issues = []jira_client_dto.Issue{ticket("OMS-1", "To Do", "acc-dev")}
	for _, step := range []time.Duration{0, 10 * time.Second, 59 * time.Minute, time.Minute} {
		now = now.Add(step)
		if err := f.svc.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	w := f.jira.windows
	if w[0] != 0 || w[1] == 0 || w[2] == 0 || w[3] != 0 {
		t.Errorf("windows = %v, want a full read first and again after an hour", w)
	}
}

func TestRunOnce_SearchErrorKeepsTheCursor(t *testing.T) {
	f := newFixture()
	f.build()
	f.jira.issues = []jira_client_dto.Issue{ticket("OMS-1", "To Do", "acc-dev")}
	if err := f.svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.jira.err = errors.New("429")
	if err := f.svc.RunOnce(context.Background()); err == nil {
		t.Fatal("want the search error")
	}
	f.jira.err = nil
	if err := f.svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.jira.windows[2] != f.jira.windows[1] {
		t.Errorf("window after a failure = %v, want unchanged %v", f.jira.windows[2], f.jira.windows[1])
	}
}
