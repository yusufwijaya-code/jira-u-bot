package jira_client_dto

import (
	"testing"
	"time"
)

func TestRuleFor_CoreTracksReviewOmsDoesNot(t *testing.T) {
	core := RuleForIssue("CORE-10923")
	if !core.IsTracked("Code Process") || !core.IsTracked("CODE REVIEW") || !core.IsTracked("code review") {
		t.Error("CORE must track Code Process and CODE REVIEW (case-insensitive)")
	}
	if core.IsTracked("Code Completed") || !core.IsDone("code completed") || !core.IsReview("CODE REVIEW") || !core.TracksReview() {
		t.Errorf("CORE rule = %+v", core)
	}

	oms := RuleForIssue("OMS-1")
	if !oms.IsTracked("Code Process") || oms.IsTracked("Code Review") || !oms.IsDone("CODE REVIEW") || oms.TracksReview() {
		t.Errorf("OMS rule = %+v", oms)
	}
}

func TestRuleFor_UnknownProjectFallsBackToLegacyRule(t *testing.T) {
	r := RuleFor("XYZ")
	if !r.IsTracked("Code Process") || r.IsTracked("CODE REVIEW") || !r.IsDone("Code Review") {
		t.Errorf("default rule = %+v", r)
	}
}

func TestProjectKeyOf(t *testing.T) {
	for in, want := range map[string]string{"CORE-1": "CORE", "OMS-12613": "OMS", "nodash": ""} {
		if got := ProjectKeyOf(in); got != want {
			t.Errorf("ProjectKeyOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTrackedIssuesJQL(t *testing.T) {
	want := `(project = CORE AND status in ("Code Process", "CODE REVIEW")) OR (project = OMS AND status in ("Code Process"))`
	if got := TrackedIssuesJQL(); got != want {
		t.Errorf("JQL = %s\nwant  %s", got, want)
	}
}

func TestTrackedSeconds_SumsEveryRunAndSkipsTheOpenOne(t *testing.T) {
	h := func(at, from, to string) ChangelogEntry {
		return ChangelogEntry{Created: at, Items: []ChangelogEntryItem{{Field: "status", FromString: from, ToString: to}}}
	}
	// Changelog arrives newest-first from Jira; order must not matter.
	histories := []ChangelogEntry{
		h("2026-11-13T09:18:00.000+0700", "CODE REVIEW", "Code Completed"),
		h("2026-11-09T15:02:00.000+0700", "Code Process", "CODE REVIEW"),
		h("2026-11-09T14:48:00.000+0700", "To Do", "Code Process"),
		h("2026-11-09T13:18:00.000+0700", "Code Process", "To Do"),
		h("2026-11-09T08:32:00.000+0700", "To Do", "Code Process"),
		{Created: "2026-11-09T08:00:00.000+0700", Items: []ChangelogEntryItem{{Field: "assignee", ToString: "x"}}},
	}
	wall := func(a, b time.Time) int64 { return int64(b.Sub(a).Seconds()) }

	got, found := TrackedSeconds(histories, RuleFor("CORE"), wall)
	// 08:32→13:18 (4h46m) + 14:48→13 Nov 09:18 (CP→REVIEW keeps the run going)
	want := int64((4*time.Hour + 46*time.Minute).Seconds()) + int64(time.Date(2026, 11, 13, 9, 18, 0, 0, time.UTC).Sub(time.Date(2026, 11, 9, 14, 48, 0, 0, time.UTC)).Seconds())
	if !found || got != want {
		t.Errorf("CORE TrackedSeconds = %d, %v; want %d", got, found, want)
	}

	// OMS: CODE REVIEW is not tracked → the second run stops at 15:02.
	got, _ = TrackedSeconds(histories, RuleFor("OMS"), wall)
	want = int64((4*time.Hour + 46*time.Minute + 14*time.Minute).Seconds())
	if got != want {
		t.Errorf("OMS TrackedSeconds = %d; want %d", got, want)
	}

	// Still open at the end → not counted (the poller owns it).
	open := []ChangelogEntry{h("2026-11-09T08:32:00.000+0700", "To Do", "Code Process")}
	if got, found := TrackedSeconds(open, RuleFor("OMS"), wall); got != 0 || !found {
		t.Errorf("open run = %d, %v; want 0, true", got, found)
	}
	if _, found := TrackedSeconds(nil, RuleFor("OMS"), wall); found {
		t.Error("empty changelog must report found=false")
	}
}
