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
