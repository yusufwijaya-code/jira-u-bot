package jira_client_dto

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
