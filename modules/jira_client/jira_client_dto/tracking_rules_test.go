package jira_client_dto

import "testing"

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
