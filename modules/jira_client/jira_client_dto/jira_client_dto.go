package jira_client_dto

import (
	"encoding/json"
	"regexp"
)

type Issue struct {
	Key    string      `json:"key"`
	Fields IssueFields `json:"fields"`
}

type IssueFields struct {
	Summary     string    `json:"summary"`
	Status      Status    `json:"status"`
	Assignee    *Assignee `json:"assignee"`
	IssueType   IssueType `json:"issuetype"`
	StoryPoints *float64  `json:"customfield_10016"`
	// StoryPointsCoreEstimate is CORE's "Story Point Estimate" field
	// (customfield_10125) — see StoryPointFieldIDCoreEstimate. Takes
	// priority over StoryPointsCore when both happen to be set. Use
	// EffectiveStoryPoints() instead of reading any of these three fields
	// directly.
	StoryPointsCoreEstimate *float64 `json:"customfield_10125"`
	// StoryPointsCore is CORE's older "Story Points" field
	// (customfield_11053) — see StoryPointFieldIDCore. Kept as a fallback:
	// some CORE tickets only have this one filled in, not Story Point
	// Estimate.
	StoryPointsCore *float64 `json:"customfield_11053"`
	// SprintRaw is the Sprint field (customfield_10020) — confirmed live
	// 2026-11-01 identical for OMS and CORE: an array of objects
	// {id, name, state, boardId, goal, startDate, endDate}, state values
	// lowercase ("active", "closed", "future"). Kept as json.RawMessage
	// because ActiveSprintName() also defends against a legacy
	// greenhopper string format that was never actually observed live but
	// costs nothing to handle defensively.
	SprintRaw json.RawMessage `json:"customfield_10020"`
}

type sprintObject struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

var legacySprintNamePattern = regexp.MustCompile(`name=([^,\]]+)`)

// ActiveSprintName extracts a sprint name from SprintRaw (Tech Spec
// "Dashboard Jira Manage", Chapter 11a) — prefers the sprint whose
// state is "active"; if none is (ticket moved after the sprint closed),
// falls back to the LAST sprint in the array. Returns false for an
// empty/null field — this is the normal case for any ticket that has
// never been assigned to a sprint, not an error.
func (f IssueFields) ActiveSprintName() (string, bool) {
	if len(f.SprintRaw) == 0 || string(f.SprintRaw) == "null" {
		return "", false
	}

	var sprints []sprintObject
	if err := json.Unmarshal(f.SprintRaw, &sprints); err == nil && len(sprints) > 0 {
		for _, s := range sprints {
			if s.State == "active" {
				return s.Name, true
			}
		}
		return sprints[len(sprints)-1].Name, true
	}

	// Defensive fallback — not a path ever observed live, but costs
	// nothing to keep (legacy greenhopper toString representation some
	// older Jira Software versions used instead of a structured array).
	if m := legacySprintNamePattern.FindStringSubmatch(string(f.SprintRaw)); len(m) == 2 {
		return m[1], true
	}

	return "", false
}

// EffectiveStoryPoints returns this issue's Story Points regardless of
// which Jira project's custom field it lives on: StoryPoints for OMS: for
// CORE, StoryPointsCoreEstimate ("Story Point Estimate") takes priority,
// falling back to StoryPointsCore ("Story Points") when Estimate is empty.
func (f IssueFields) EffectiveStoryPoints() *float64 {
	if f.StoryPoints != nil {
		return f.StoryPoints
	}
	if f.StoryPointsCoreEstimate != nil {
		return f.StoryPointsCoreEstimate
	}
	return f.StoryPointsCore
}

type Status struct {
	Name string `json:"name"`
}

type Assignee struct {
	// AccountID is always populated by Jira, unlike EmailAddress — this org
	// hides emailAddress on assignee objects for any account but the API
	// token's own (confirmed live). Eligibility matching must key off this
	// field, not EmailAddress.
	AccountID    string `json:"accountId"`
	EmailAddress string `json:"emailAddress"`
	DisplayName  string `json:"displayName"`
}

type IssueType struct {
	Name string `json:"name"`
}

type SearchResponse struct {
	Issues []Issue `json:"issues"`
}

type ChangelogEntry struct {
	Created string               `json:"created"`
	Items   []ChangelogEntryItem `json:"items"`
}

type ChangelogEntryItem struct {
	Field      string `json:"field"`
	ToString   string `json:"toString"`
	FromString string `json:"fromString"`
}

type ChangelogResponse struct {
	Changelog struct {
		Histories []ChangelogEntry `json:"histories"`
	} `json:"changelog"`
}

type UserSearchResult struct {
	AccountID    string `json:"accountId"`
	EmailAddress string `json:"emailAddress"`
}

// Transition is one entry from GET /issue/{key}/transitions — the set of
// moves actually available from the issue's CURRENT status, as defined by
// its project's workflow. Workflows vary per project and issue type (e.g.
// OMS and CORE use differently-capitalized status names, "Code Review" vs
// "CODE REVIEW"), so callers must match To.Name case-insensitively rather
// than assuming a transition exists or hardcoding its ID.
type Transition struct {
	ID string `json:"id"`
	To Status `json:"to"`
}

type TransitionsResponse struct {
	Transitions []Transition `json:"transitions"`
}

// TransitionRequest is the body for POST /issue/{key}/transitions.
type TransitionRequest struct {
	Transition TransitionID `json:"transition"`
}

type TransitionID struct {
	ID string `json:"id"`
}

// Credential is a per-user Jira Basic Auth pair (Tech Spec "Dashboard Jira
// Manage", Chapter 9e) — passed to the *As methods on JiraClientService to
// act as that user instead of the central service account. A nil
// *Credential means "use the central credential" throughout this package.
type Credential struct {
	Email    string
	APIToken string
}
