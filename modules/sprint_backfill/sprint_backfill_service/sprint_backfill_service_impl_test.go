package sprint_backfill_service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/issue_session/issue_session_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/registration/registration_dto"
)

const activeSprint = `[{"id": 7988, "name": "Core Sprint 23 2026", "state": "active", "boardId": 26}]`
const closedSprint = `[{"id": 2393, "name": "Core Sprint 13", "state": "closed", "boardId": 26}]`

func sp(v float64) *float64 { return &v }

func ticket(key, status, sprint string, points *float64, assignee string) jira_client_dto.Issue {
	return jira_client_dto.Issue{Key: key, Fields: jira_client_dto.IssueFields{
		Summary: key + " summary", Status: jira_client_dto.Status{Name: status},
		SprintRaw: json.RawMessage(sprint), Numbers: map[string]*float64{"customfield_10125": points},
		Assignee: &jira_client_dto.Assignee{AccountID: assignee},
	}}
}

// cpThenReview: Code Process Mon 08:30 → CODE REVIEW Mon 12:00 → Code Completed Tue 10:30.
// CORE counts both → Mon 08:30–17:30 minus lunch (8h) + Tue 08:30–10:30 (2h) = 10h.
func cpThenReview() []jira_client_dto.ChangelogEntry {
	h := func(at, from, to string) jira_client_dto.ChangelogEntry {
		return jira_client_dto.ChangelogEntry{Created: at, Items: []jira_client_dto.ChangelogEntryItem{{Field: "status", FromString: from, ToString: to}}}
	}
	return []jira_client_dto.ChangelogEntry{
		h("2026-11-09T08:30:00.000+0700", "To Do", "Code Process"),
		h("2026-11-09T12:00:00.000+0700", "Code Process", "CODE REVIEW"),
		h("2026-11-10T10:30:00.000+0700", "CODE REVIEW", "Code Completed"),
	}
}

type fakeJira struct {
	sprint      []jira_client_dto.Issue
	related     []jira_client_dto.Issue
	changelogs  map[string][]jira_client_dto.ChangelogEntry
	askedKeys   []string
	searchErr   error
	changelogOf []string
}

func (f *fakeJira) SearchActiveSprintIssuesByAssignee(ctx context.Context, cred *jira_client_dto.Credential, accountID string) ([]jira_client_dto.Issue, error) {
	return f.sprint, f.searchErr
}
func (f *fakeJira) SearchIssuesByKeysForAssignee(ctx context.Context, cred *jira_client_dto.Credential, keys []string, accountID string) ([]jira_client_dto.Issue, error) {
	f.askedKeys = keys
	return f.related, nil
}
func (f *fakeJira) GetChangelog(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) ([]jira_client_dto.ChangelogEntry, error) {
	f.changelogOf = append(f.changelogOf, issueKey)
	return f.changelogs[issueKey], nil
}

type fakeUsers struct{ users []registration_dto.User }

func (f *fakeUsers) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	for _, u := range f.users {
		if u.Email == email {
			return &u, true, nil
		}
	}
	return nil, false, nil
}
func (f *fakeUsers) FindAll(ctx context.Context) ([]registration_dto.User, error) {
	return f.users, nil
}

type fakeSessions struct {
	existing map[string]bool
	inserted map[string]issue_session_dto.IssueSession
	updated  map[string]int64
}

func (f *fakeSessions) UpdateAccumulatedSeconds(ctx context.Context, issueKey string, seconds int64) error {
	if f.updated == nil {
		f.updated = map[string]int64{}
	}
	f.updated[issueKey] = seconds
	return nil
}

func (f *fakeSessions) SessionExists(ctx context.Context, issueKey string) (bool, error) {
	return f.existing[issueKey], nil
}
func (f *fakeSessions) InsertClosedSession(ctx context.Context, s issue_session_dto.IssueSession) (bool, error) {
	if f.inserted == nil {
		f.inserted = map[string]issue_session_dto.IssueSession{}
	}
	f.inserted[s.IssueKey] = s
	return true, nil
}

func newService(jira *fakeJira, users *fakeUsers, sessions *fakeSessions) *sprintBackfillServiceImpl {
	return NewSprintBackfillService(jira, users, nil, sessions, business_hours_service.NewBusinessHoursService(), business_hours_dto.HolidayCalendar{}).(*sprintBackfillServiceImpl)
}

var yusuf = registration_dto.User{UserID: 1, Email: "yusuf@esb.co.id", JiraAccountID: "acc-1", FlagActive: true}

func TestRunForEmail_BackfillsFinishedSprintTicketsAndTheirSubtasks(t *testing.T) {
	parent := ticket("CORE-10923", "Code Completed", activeSprint, sp(16), "acc-1")
	parent.Fields.Subtasks = []jira_client_dto.IssueRef{{Key: "CORE-10955"}, {Key: "CORE-10956"}, {Key: "CORE-10957"}}
	jira := &fakeJira{
		sprint: []jira_client_dto.Issue{
			parent,
			ticket("CORE-11000", "Code Process", activeSprint, sp(2), "acc-1"), // in progress: poller's job
			ticket("CORE-11001", "Done", activeSprint, nil, "acc-1"),           // no SP
			ticket("CORE-11002", "Done", activeSprint, sp(1), "acc-1"),         // already has a session
			ticket("CORE-11003", "To Do", activeSprint, sp(1), "acc-1"),        // never tracked
		},
		related: []jira_client_dto.Issue{
			ticket("CORE-10955", "Done", activeSprint, sp(4), "acc-1"),
			ticket("CORE-10956", "Done", closedSprint, sp(1), "acc-1"),   // not in an active sprint
			ticket("CORE-10957", "Done", activeSprint, sp(4.5), "acc-9"), // someone else's
		},
		changelogs: map[string][]jira_client_dto.ChangelogEntry{
			"CORE-10923": cpThenReview(),
			"CORE-10955": cpThenReview(),
			"CORE-11002": cpThenReview(),
		},
	}
	sessions := &fakeSessions{existing: map[string]bool{"CORE-11002": true}}
	svc := newService(jira, &fakeUsers{users: []registration_dto.User{yusuf}}, sessions)

	added, updated, err := svc.RunForEmail(context.Background(), "yusuf@esb.co.id")
	if err != nil || added != 2 || updated != 1 {
		t.Fatalf("RunForEmail = %d added, %d updated, %v; want 2 (CORE-10923, CORE-10955), 1 (CORE-11002)", added, updated, err)
	}
	if got := sessions.updated["CORE-11002"]; got != 10*3600 {
		t.Errorf("existing CORE-11002 recalculated to %d, want %d (Code Process + CODE REVIEW)", got, 10*3600)
	}
	if _, ok := sessions.updated["CORE-10923"]; ok {
		t.Error("a session the backfill just inserted must not be recalculated again")
	}
	if !reflect.DeepEqual(jira.askedKeys, []string{"CORE-10955", "CORE-10956", "CORE-10957"}) {
		t.Errorf("related keys asked = %v", jira.askedKeys)
	}

	got := sessions.inserted["CORE-10923"]
	if got.AccumulatedSeconds != 10*3600 || got.StoryPoints != 16 || got.SprintName != "Core Sprint 23 2026" ||
		got.AssigneeEmail != "yusuf@esb.co.id" || got.ProjectKey != "CORE" || *got.BoardID != 26 || *got.SprintID != 7988 {
		t.Errorf("CORE-10923 session = %+v (acc %d), want 10h of business time in Core Sprint 23 2026", got, got.AccumulatedSeconds)
	}
	if _, ok := sessions.inserted["CORE-10955"]; !ok {
		t.Error("subtask CORE-10955 (active sprint, mine) must be backfilled")
	}

	// Changelogs are only fetched for real candidates, once each.
	sort.Strings(jira.changelogOf)
	if !reflect.DeepEqual(jira.changelogOf, []string{"CORE-10923", "CORE-10955", "CORE-11002", "CORE-11003"}) {
		t.Errorf("changelog fetched for %v", jira.changelogOf)
	}
}

func TestRunForEmail_UnknownUserAndSearchFailure(t *testing.T) {
	svc := newService(&fakeJira{}, &fakeUsers{}, &fakeSessions{})
	if added, updated, err := svc.RunForEmail(context.Background(), "nobody@esb.co.id"); added != 0 || updated != 0 || err != nil {
		t.Errorf("unknown user = %d, %d, %v; want 0, 0, nil", added, updated, err)
	}
	svc = newService(&fakeJira{searchErr: errors.New("jira down")}, &fakeUsers{users: []registration_dto.User{yusuf}}, &fakeSessions{})
	if _, _, err := svc.RunForEmail(context.Background(), "yusuf@esb.co.id"); err == nil {
		t.Error("a failed sprint search must be reported to the button caller")
	}
}

func TestRunAll_SkipsInactiveUsersAndKeepsGoing(t *testing.T) {
	jira := &fakeJira{
		sprint:     []jira_client_dto.Issue{ticket("CORE-1", "Done", activeSprint, sp(1), "")},
		changelogs: map[string][]jira_client_dto.ChangelogEntry{"CORE-1": cpThenReview()},
	}
	users := &fakeUsers{users: []registration_dto.User{
		yusuf,
		{UserID: 2, Email: "off@esb.co.id", JiraAccountID: "acc-2", FlagActive: false},
		{UserID: 3, Email: "nojira@esb.co.id", FlagActive: true},
	}}
	sessions := &fakeSessions{}
	if added := newService(jira, users, sessions).RunAll(context.Background()); added != 1 {
		t.Errorf("RunAll = %d, want 1 (only the active user with a Jira account)", added)
	}
}

// A session recorded by an older rule (CORE: stopped at CODE REVIEW) is
// rewritten by the daily run, not only the ticket the poller never saw.
func TestRunAll_RecalculatesExistingSessions(t *testing.T) {
	jira := &fakeJira{
		sprint:     []jira_client_dto.Issue{ticket("CORE-10923", "Done", activeSprint, sp(16), "acc-1")},
		changelogs: map[string][]jira_client_dto.ChangelogEntry{"CORE-10923": cpThenReview()},
	}
	sessions := &fakeSessions{existing: map[string]bool{"CORE-10923": true}}
	if added := newService(jira, &fakeUsers{users: []registration_dto.User{yusuf}}, sessions).RunAll(context.Background()); added != 0 {
		t.Errorf("RunAll added %d, want 0 (the session exists)", added)
	}
	if got := sessions.updated["CORE-10923"]; got != 10*3600 {
		t.Errorf("CORE-10923 accumulated = %d, want %d", got, 10*3600)
	}
}

func TestNextDailyRun_Is0700WIB(t *testing.T) {
	cases := map[string]string{
		"2026-11-15T06:59:00+07:00": "2026-11-15T07:00:00+07:00",
		"2026-11-15T07:00:00+07:00": "2026-11-16T07:00:00+07:00",
		"2026-11-15T23:30:00+07:00": "2026-11-16T07:00:00+07:00",
	}
	for in, want := range cases {
		now, _ := time.Parse(time.RFC3339, in)
		if got := nextDailyRun(now).Format(time.RFC3339); got != want {
			t.Errorf("nextDailyRun(%s) = %s, want %s", in, got, want)
		}
	}
}

// Tech Spec "Papan Tiket & Cuti", AC4.2/AC4.6: leave recorded on a day the
// ticket was already worked removes that day from the stored time.
func TestRecalculateForEmail_ExcludesLeaveDayFromStoredTime(t *testing.T) {
	jira := &fakeJira{
		sprint: []jira_client_dto.Issue{
			ticket("CORE-10923", "Code Completed", activeSprint, sp(16), "acc-1"),
			ticket("CORE-11003", "Done", activeSprint, sp(1), "acc-1"), // no session: left to the backfill
			ticket("CORE-11004", "Done", closedSprint, sp(1), "acc-1"), // not in an active sprint (AC4.3)
		},
		changelogs: map[string][]jira_client_dto.ChangelogEntry{
			"CORE-10923": cpThenReview(),
			"CORE-11003": cpThenReview(),
			"CORE-11004": cpThenReview(),
		},
	}
	sessions := &fakeSessions{existing: map[string]bool{"CORE-10923": true, "CORE-11004": true}}
	// Leave on Mon 9 Nov: only Tue 08:30–10:30 (2h) is left of the 10h.
	leave := business_hours_dto.HolidayCalendar{Holidays: map[string]bool{"2026-11-09": true}}
	svc := NewSprintBackfillService(jira, &fakeUsers{users: []registration_dto.User{yusuf}}, nil, sessions,
		business_hours_service.NewBusinessHoursService(), leave)

	n, err := svc.RecalculateForEmail(context.Background(), "yusuf@esb.co.id")
	if err != nil || n != 1 {
		t.Fatalf("RecalculateForEmail = %d, %v; want 1", n, err)
	}
	if got := sessions.updated["CORE-10923"]; got != 2*3600 {
		t.Errorf("CORE-10923 accumulated = %d, want %d", got, 2*3600)
	}
	if _, ok := sessions.updated["CORE-11003"]; ok {
		t.Error("a ticket without a session must not be written by the recalculation")
	}
	if _, ok := sessions.updated["CORE-11004"]; ok {
		t.Error("a ticket outside the active sprint must not be touched")
	}
	if len(sessions.inserted) != 0 {
		t.Errorf("recalculation must never insert sessions, got %v", sessions.inserted)
	}
}

// The open run of a ticket still in a tracked status stays with the poller:
// only finished runs are written.
func TestRecalculateForEmail_SkipsTheRunningPeriod(t *testing.T) {
	h := func(at, from, to string) jira_client_dto.ChangelogEntry {
		return jira_client_dto.ChangelogEntry{Created: at, Items: []jira_client_dto.ChangelogEntryItem{{Field: "status", FromString: from, ToString: to}}}
	}
	jira := &fakeJira{
		sprint: []jira_client_dto.Issue{ticket("OMS-1", "Code Process", activeSprint, sp(8), "acc-1")},
		changelogs: map[string][]jira_client_dto.ChangelogEntry{"OMS-1": {
			h("2026-11-09T08:30:00.000+0700", "To Do", "Code Process"),
			h("2026-11-09T17:30:00.000+0700", "Code Process", "To Do"),
			h("2026-11-10T08:30:00.000+0700", "To Do", "Code Process"),
		}},
	}
	sessions := &fakeSessions{existing: map[string]bool{"OMS-1": true}}
	svc := newService(jira, &fakeUsers{users: []registration_dto.User{yusuf}}, sessions)

	if _, err := svc.RecalculateForEmail(context.Background(), "yusuf@esb.co.id"); err != nil {
		t.Fatal(err)
	}
	if got := sessions.updated["OMS-1"]; got != 8*3600 {
		t.Errorf("OMS-1 accumulated = %d, want only the finished Monday run (%d)", got, 8*3600)
	}
}
