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
}

type Status struct {
	Name string `json:"name"`
}

type Assignee struct {
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
