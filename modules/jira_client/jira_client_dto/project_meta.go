package jira_client_dto

// Jira project data the Admin "Aturan Board" form picks from (Tech Spec
// "Aturan Board (Admin CRUD)").

// ProjectInfo is GET /rest/api/3/project/{key}.
type ProjectInfo struct {
	Key        string            `json:"key"`
	Name       string            `json:"name"`
	AvatarURLs map[string]string `json:"avatarUrls"`
}

// AvatarURL is the project's 48x48 avatar ("" when Jira sends none).
func (p ProjectInfo) AvatarURL() string { return p.AvatarURLs["48x48"] }

// ProjectIssueTypeStatuses is one entry of GET /rest/api/3/project/{key}/statuses:
// the statuses one issue type's workflow uses.
type ProjectIssueTypeStatuses struct {
	Name     string   `json:"name"`
	Subtask  bool     `json:"subtask"`
	Statuses []Status `json:"statuses"`
}

// JiraField is one entry of GET /rest/api/3/field.
type JiraField struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Custom bool        `json:"custom"`
	Schema FieldSchema `json:"schema"`
}

// NumberField is a numeric custom field an admin can pick as a Story Point
// field; StoryPointLike = its name contains "story point".
type NumberField struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	StoryPointLike bool   `json:"storyPointLike"`
}
