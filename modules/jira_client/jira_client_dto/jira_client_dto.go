package jira_client_dto

import (
	"encoding/json"
	"regexp"
	"strings"
)

type Issue struct {
	Key    string      `json:"key"`
	Fields IssueFields `json:"fields"`
}

type IssueFields struct {
	Summary   string    `json:"summary"`
	Status    Status    `json:"status"`
	Assignee  *Assignee `json:"assignee"`
	IssueType IssueType `json:"issuetype"`
	// Parent, Subtasks and IssueLinks are only requested by the dashboard's
	// active-sprint search (Tech Spec "Subtask & Linked Item Branches").
	Parent     *IssueRef   `json:"parent,omitempty"`
	Subtasks   []IssueRef  `json:"subtasks,omitempty"`
	IssueLinks []IssueLink `json:"issuelinks,omitempty"`
	// SprintRaw is the Sprint field (customfield_10020) — confirmed live
	// 2026-11-01 identical for OMS and CORE: an array of objects
	// {id, name, state, boardId, goal, startDate, endDate}, state values
	// lowercase ("active", "closed", "future"). Kept as json.RawMessage
	// because ActiveSprintName() also defends against a legacy
	// greenhopper string format that was never actually observed live but
	// costs nothing to handle defensively.
	SprintRaw json.RawMessage `json:"customfield_10020"`
	// Numbers holds every numeric custom field of the response by ID
	// (null ones are left out). Story Points live here — which field
	// depends on the project's tracking rule (Tech Spec "Aturan Board");
	// read them through Issue.StoryPoints().
	Numbers map[string]*float64 `json:"-"`
}

// UnmarshalJSON decodes the regular fields, then keeps every numeric
// customfield_* in Numbers — the Story Point fields are configured per
// project at runtime, so they can't be struct tags.
func (f *IssueFields) UnmarshalJSON(data []byte) error {
	type plain IssueFields
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for id, value := range raw {
		if !strings.HasPrefix(id, "customfield_") || strings.TrimSpace(string(value)) == "null" {
			continue
		}
		var n float64
		if json.Unmarshal(value, &n) != nil {
			continue
		}
		if p.Numbers == nil {
			p.Numbers = map[string]*float64{}
		}
		p.Numbers[id] = &n
	}
	*f = IssueFields(p)
	return nil
}

// FirstNumber returns the first non-null numeric custom field among ids.
func (f IssueFields) FirstNumber(ids ...string) *float64 {
	for _, id := range ids {
		if v := f.Numbers[id]; v != nil {
			return v
		}
	}
	return nil
}

type sprintObject struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	BoardID   int64  `json:"boardId"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// SprintRef is one sprint as Jira reports it — on an issue's Sprint field or
// from the Agile API.
type SprintRef struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	BoardID   int64  `json:"boardId"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// ActiveSprint returns the issue's sprint whose state is "active" — the
// board that sprint belongs to is the ticket's board (Tech Spec "Per-Board
// Settings & Active Sprint Dashboard"). Unlike ActiveSprintName there is no
// fallback to a closed sprint: no active sprint means no board.
func (f IssueFields) ActiveSprint() (SprintRef, bool) {
	if len(f.SprintRaw) == 0 || string(f.SprintRaw) == "null" {
		return SprintRef{}, false
	}
	var sprints []sprintObject
	if err := json.Unmarshal(f.SprintRaw, &sprints); err != nil {
		return SprintRef{}, false
	}
	for _, s := range sprints {
		if s.State == "active" && s.BoardID > 0 {
			return SprintRef(s), true
		}
	}
	return SprintRef{}, false
}

// BoardInfo is GET /rest/agile/1.0/board/{id}. Jira boards have no avatar
// of their own — location.avatarURI is the board's project avatar, which is
// what Jira shows next to the board too.
type BoardInfo struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Location struct {
		ProjectKey string `json:"projectKey"`
		AvatarURI  string `json:"avatarURI"`
	} `json:"location"`
}

// SprintPage is GET /rest/agile/1.0/board/{id}/sprint.
type SprintPage struct {
	Values []SprintRef `json:"values"`
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

// StoryPoints returns the issue's Story Points: the first non-null of its
// project's StoryPointFields (priority order). A project without an active
// rule reads every active rule's fields, like the old global coalesce.
func (i Issue) StoryPoints() *float64 {
	fields := RuleForIssue(i.Key).StoryPointFields
	if len(fields) == 0 {
		fields = StoryPointFieldIDs()
	}
	return i.Fields.FirstNumber(fields...)
}

type Status struct {
	// ID is the status id (a transition's target, matched against workflow rules).
	ID   string `json:"id,omitempty"`
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
	// NextPageToken is set by /search/jql while more pages remain.
	NextPageToken string `json:"nextPageToken"`
}

type ChangelogEntry struct {
	Created string               `json:"created"`
	Author  *ChangelogAuthor     `json:"author"`
	Items   []ChangelogEntryItem `json:"items"`
}

// ChangelogAuthor is who made a change (Tech Spec "Notifikasi Perubahan
// Status Tiket": "digeser oleh …").
type ChangelogAuthor struct {
	DisplayName string `json:"displayName"`
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
	ID   string `json:"id"`
	Name string `json:"name"` // workflow wording, e.g. "Start Development"
	To   Status `json:"to"`
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
