package jira_client_dto

import "testing"

func floatPtr(v float64) *float64 { return &v }

func TestEffectiveStoryPoints_UsesOMSFieldWhenPresent(t *testing.T) {
	f := IssueFields{StoryPoints: floatPtr(3)}

	got := f.EffectiveStoryPoints()

	if got == nil || *got != 3 {
		t.Errorf("EffectiveStoryPoints() = %v, want 3", got)
	}
}

func TestEffectiveStoryPoints_PrefersCoreStoryPointEstimateOverStoryPoints(t *testing.T) {
	// CORE issues can carry "Story Point Estimate" (customfield_10125) and
	// "Story Points" (customfield_11053) as two separate fields — when both
	// are populated, Story Point Estimate wins.
	f := IssueFields{StoryPointsCoreEstimate: floatPtr(3), StoryPointsCore: floatPtr(5)}

	got := f.EffectiveStoryPoints()

	if got == nil || *got != 3 {
		t.Errorf("EffectiveStoryPoints() = %v, want 3 (Story Point Estimate takes priority)", got)
	}
}

func TestEffectiveStoryPoints_FallsBackToCoreStoryPointsWhenEstimateMissing(t *testing.T) {
	f := IssueFields{StoryPointsCoreEstimate: nil, StoryPointsCore: floatPtr(5)}

	got := f.EffectiveStoryPoints()

	if got == nil || *got != 5 {
		t.Errorf("EffectiveStoryPoints() = %v, want 5 (fallback to Story Points when Story Point Estimate is empty)", got)
	}
}

func TestEffectiveStoryPoints_ReturnsNilWhenNoFieldIsPopulated(t *testing.T) {
	f := IssueFields{}

	got := f.EffectiveStoryPoints()

	if got != nil {
		t.Errorf("EffectiveStoryPoints() = %v, want nil", got)
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
