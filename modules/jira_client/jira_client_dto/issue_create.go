package jira_client_dto

// Createmeta / create DTOs (Tech Spec "Papan Tiket Revamp & Buat Card Jira", Phase 2).
// Jira answers createmeta lists under "issueTypes"/"fields" or "values"
// depending on the deployment, so both keys are accepted.

type CreateMetaIssueType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Subtask bool   `json:"subtask"`
}

type CreateMetaIssueTypesPage struct {
	IssueTypes []CreateMetaIssueType `json:"issueTypes"`
	Values     []CreateMetaIssueType `json:"values"`
}

func (p CreateMetaIssueTypesPage) Items() []CreateMetaIssueType {
	if len(p.IssueTypes) > 0 {
		return p.IssueTypes
	}
	return p.Values
}

type FieldSchema struct {
	Type   string `json:"type"`
	Items  string `json:"items"`
	Custom string `json:"custom"`
	System string `json:"system"`
}

type AllowedValue struct {
	ID    string `json:"id"`
	Value string `json:"value"`
	Name  string `json:"name"`
}

// Label is what a person reads: option fields use "value", others "name".
func (a AllowedValue) Label() string {
	if a.Value != "" {
		return a.Value
	}
	return a.Name
}

type CreateMetaField struct {
	FieldID       string         `json:"fieldId"`
	Key           string         `json:"key"`
	Name          string         `json:"name"`
	Required      bool           `json:"required"`
	Schema        FieldSchema    `json:"schema"`
	AllowedValues []AllowedValue `json:"allowedValues"`
}

// ID is fieldId, falling back to key on older responses.
func (f CreateMetaField) ID() string {
	if f.FieldID != "" {
		return f.FieldID
	}
	return f.Key
}

type CreateMetaFieldsPage struct {
	Fields []CreateMetaField `json:"fields"`
	Values []CreateMetaField `json:"values"`
}

func (p CreateMetaFieldsPage) Items() []CreateMetaField {
	if len(p.Fields) > 0 {
		return p.Fields
	}
	return p.Values
}

type CreateIssueRequest struct {
	Fields map[string]any `json:"fields"`
}

type CreateIssueResponse struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// JiraErrorBody is Jira's 4xx body: per-field errors + general messages.
type JiraErrorBody struct {
	ErrorMessages []string          `json:"errorMessages"`
	Errors        map[string]string `json:"errors"`
}

type AssignableUser struct {
	AccountID   string            `json:"accountId"`
	DisplayName string            `json:"displayName"`
	Active      bool              `json:"active"`
	AvatarURLs  map[string]string `json:"avatarUrls"`
}

type IssueBrief struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Project struct {
			Key string `json:"key"`
		} `json:"project"`
		IssueType struct {
			Subtask bool `json:"subtask"`
		} `json:"issuetype"`
		Subtasks []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
			} `json:"fields"`
		} `json:"subtasks"`
	} `json:"fields"`
}

type Myself struct {
	AccountID string `json:"accountId"`
}
