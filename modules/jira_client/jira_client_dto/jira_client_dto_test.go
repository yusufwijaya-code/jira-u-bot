package jira_client_dto

import (
	"encoding/json"
	"testing"
)

func floatPtr(v float64) *float64 { return &v }

func TestIssueFields_UnmarshalKeepsNumericCustomFields(t *testing.T) {
	raw := `{"key":"CORE-1","fields":{"summary":"x","customfield_10125":3,"customfield_11053":null,
		"customfield_10020":[{"id":1,"state":"active","boardId":26}],"customfield_12001":"text"}}`
	var issue Issue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatal(err)
	}
	if issue.Fields.Summary != "x" || len(issue.Fields.SprintRaw) == 0 {
		t.Errorf("regular fields lost: %+v", issue.Fields)
	}
	if v := issue.Fields.Numbers["customfield_10125"]; v == nil || *v != 3 {
		t.Errorf("customfield_10125 = %v, want 3", v)
	}
	for _, id := range []string{"customfield_11053", "customfield_10020", "customfield_12001"} {
		if _, ok := issue.Fields.Numbers[id]; ok {
			t.Errorf("%s must not be kept (null / not a number)", id)
		}
	}
}

// legacyEffectiveStoryPoints is the old global coalesce (10016 → 10125 →
// 11053), kept only to prove the per-project rules give the same result.
func legacyEffectiveStoryPoints(n map[string]*float64) *float64 {
	for _, id := range []string{"customfield_10016", "customfield_10125", "customfield_11053"} {
		if v := n[id]; v != nil {
			return v
		}
	}
	return nil
}

func TestIssueStoryPoints_ParityWithTheLegacyCoalesce(t *testing.T) {
	// Live data: OMS never fills CORE's fields and vice versa (constants.go).
	cases := []struct {
		key     string
		numbers map[string]*float64
	}{
		{"OMS-1", map[string]*float64{"customfield_10016": floatPtr(3)}},
		{"OMS-2", nil},
		{"CORE-1", map[string]*float64{"customfield_10125": floatPtr(3), "customfield_11053": floatPtr(5)}},
		{"CORE-2", map[string]*float64{"customfield_11053": floatPtr(5)}},
		{"CORE-3", map[string]*float64{"customfield_10125": floatPtr(2)}},
		{"CORE-4", nil},
	}
	for _, c := range cases {
		got := Issue{Key: c.key, Fields: IssueFields{Numbers: c.numbers}}.StoryPoints()
		want := legacyEffectiveStoryPoints(c.numbers)
		if (got == nil) != (want == nil) || (got != nil && *got != *want) {
			t.Errorf("%s: StoryPoints() = %v, legacy = %v", c.key, got, want)
		}
	}
}

func TestIssueStoryPoints_FollowsTheProjectRuleOrder(t *testing.T) {
	withRules(t, TrackingRule{ProjectKey: "POS", TrackedStatuses: []string{"In Progress"}, DoneStatus: "Done",
		StoryPointFields: []string{"customfield_2", "customfield_1"}})
	issue := Issue{Key: "POS-1", Fields: IssueFields{Numbers: map[string]*float64{"customfield_1": floatPtr(1), "customfield_2": floatPtr(2)}}}
	if got := issue.StoryPoints(); got == nil || *got != 2 {
		t.Errorf("POS-1 = %v, want 2 (customfield_2 has priority)", got)
	}
	// A project without a rule reads every active rule's fields.
	unknown := Issue{Key: "HR-1", Fields: IssueFields{Numbers: map[string]*float64{"customfield_1": floatPtr(1)}}}
	if got := unknown.StoryPoints(); got == nil || *got != 1 {
		t.Errorf("HR-1 = %v, want 1", got)
	}
}

func TestActiveSprintName_RealOMSFixture(t *testing.T) {
	// Captured live from esb-id.atlassian.net 2026-11-01 (OMS-12548).
	f := IssueFields{SprintRaw: []byte(`[{"id":8293,"name":"OMS Sprint 22 - 2026","state":"active","boardId":28,"goal":"","startDate":"2026-10-26T03:15:31.542Z","endDate":"2026-11-08T17:00:00.000Z"}]`)}

	name, found := f.ActiveSprintName()
	if !found || name != "OMS Sprint 22 - 2026" {
		t.Errorf("ActiveSprintName() = %q, %v, want %q, true", name, found, "OMS Sprint 22 - 2026")
	}
}

func TestActiveSprintName_RealCoreFixture(t *testing.T) {
	// Captured live from esb-id.atlassian.net 2026-11-01 (CORE-10881).
	f := IssueFields{SprintRaw: []byte(`[{"id":7987,"name":"Core Sprint 22 2026","state":"active","boardId":26,"goal":"","startDate":"2026-10-26T03:06:47.518Z","endDate":"2026-11-06T17:21:46.000Z"}]`)}

	name, found := f.ActiveSprintName()
	if !found || name != "Core Sprint 22 2026" {
		t.Errorf("ActiveSprintName() = %q, %v, want %q, true", name, found, "Core Sprint 22 2026")
	}
}

func TestActiveSprintName_FallsBackToLastSprintWhenNoneActive(t *testing.T) {
	f := IssueFields{SprintRaw: []byte(`[{"name":"Sprint 18","state":"closed"},{"name":"Sprint 19","state":"closed"}]`)}

	name, found := f.ActiveSprintName()
	if !found || name != "Sprint 19" {
		t.Errorf("ActiveSprintName() = %q, %v, want %q, true (last sprint in array)", name, found, "Sprint 19")
	}
}

func TestActiveSprintName_FalseWhenFieldNull(t *testing.T) {
	f := IssueFields{SprintRaw: []byte(`null`)}

	_, found := f.ActiveSprintName()
	if found {
		t.Error("ActiveSprintName() found = true, want false for a ticket never assigned to a sprint")
	}
}

func TestActiveSprintName_FalseWhenFieldEmpty(t *testing.T) {
	f := IssueFields{}

	_, found := f.ActiveSprintName()
	if found {
		t.Error("ActiveSprintName() found = true, want false for an unset field")
	}
}

func TestActiveSprintName_LegacyStringFormatFallback(t *testing.T) {
	// Defensive fallback — never observed live, but kept cheap to handle.
	f := IssueFields{SprintRaw: []byte(`"com.atlassian.greenhopper.service.sprint.Sprint@abc123[id=61,rapidViewId=10,state=ACTIVE,name=Sprint 20,startDate=...]"`)}

	name, found := f.ActiveSprintName()
	if !found || name != "Sprint 20" {
		t.Errorf("ActiveSprintName() = %q, %v, want %q, true", name, found, "Sprint 20")
	}
}

func TestActiveSprint_ReturnsActiveSprintWithBoard(t *testing.T) {
	f := IssueFields{SprintRaw: json.RawMessage(`[
		{"id": 8293, "name": "OMS Sprint 22 - 2026", "state": "closed", "boardId": 28},
		{"id": 8294, "name": "OMS Sprint 23 - 2026", "state": "active", "boardId": 28, "startDate": "2026-11-09T01:00:00.000Z", "endDate": "2026-11-22T10:00:00.000Z"}
	]`)}
	got, ok := f.ActiveSprint()
	if !ok || got.ID != 8294 || got.BoardID != 28 || got.Name != "OMS Sprint 23 - 2026" {
		t.Errorf("ActiveSprint = %+v, %v; want sprint 8294 on board 28", got, ok)
	}
}

func TestActiveSprint_FalseWithoutActiveSprint(t *testing.T) {
	for name, raw := range map[string]string{
		"null":        `null`,
		"empty":       ``,
		"only closed": `[{"id": 1, "name": "S1", "state": "closed", "boardId": 28}]`,
		"no board":    `[{"id": 2, "name": "S2", "state": "active"}]`,
		"legacy":      `["com.atlassian.greenhopper...[name=S1,state=ACTIVE]"]`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := (IssueFields{SprintRaw: json.RawMessage(raw)}).ActiveSprint(); ok {
				t.Errorf("ActiveSprint(%s) = true, want false", raw)
			}
		})
	}
}
