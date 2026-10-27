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
	// StoryPointsCore is the CORE project's Story Points field
	// (customfield_11053) — see StoryPointFieldIDCore. Use
	// EffectiveStoryPoints() instead of reading StoryPoints/StoryPointsCore
	// directly.
	StoryPointsCore *float64 `json:"customfield_11053"`
}

// EffectiveStoryPoints returns this issue's Story Points regardless of
// which Jira project's custom field it lives on (StoryPoints for OMS,
// StoryPointsCore for CORE) — confirmed live that an issue never has both
// populated.
func (f IssueFields) EffectiveStoryPoints() *float64 {
	if f.StoryPoints != nil {
		return f.StoryPoints
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
