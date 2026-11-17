package jira_client_dto

import "strings"

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
	// Disabled (options) / Archived (versions) values can't be picked.
	Disabled bool `json:"disabled"`
	Archived bool `json:"archived"`
	// Children: the second level of a cascading select (option-with-child).
	Children []AllowedValue `json:"children"`
}

// Pickable reports whether a person may still choose this value.
func (a AllowedValue) Pickable() bool { return a.ID != "" && !a.Disabled && !a.Archived }

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

// Form kinds Buat Card can fill (Tech Spec "Buat Card: field tambahan").
const (
	FieldKindText     = "text"
	FieldKindTextarea = "textarea"
	FieldKindNumber   = "number"
	FieldKindSelect   = "select"
	FieldKindCascade  = "cascade"
	FieldKindDate     = "date"
	FieldKindLabels   = "labels"
)

// FormKind maps a createmeta schema to the Buat Card control that fills it;
// kind "" = not fillable from Nudge (user pickers, attachments, links,
// time tracking, datetime, dev-integration fields…).
func (s FieldSchema) FormKind() (kind string, multi bool) {
	switch s.Type {
	case "string":
		// Multi-line text takes ADF on API v3 — a separate kind.
		if strings.HasSuffix(s.Custom, ":textarea") || s.System == "environment" {
			return FieldKindTextarea, false
		}
		return FieldKindText, false
	case "number":
		return FieldKindNumber, false
	case "option", "priority", "version", "component":
		return FieldKindSelect, false
	case "option-with-child":
		return FieldKindCascade, false
	case "date":
		return FieldKindDate, false
	case "array":
		switch s.Items {
		case "option", "version", "component":
			return FieldKindSelect, true
		case "string":
			return FieldKindLabels, true
		}
	}
	return "", false
}

// NeedsOptions: select/cascade fields are fillable only with allowed values.
func NeedsOptions(kind string) bool { return kind == FieldKindSelect || kind == FieldKindCascade }

// FormKind is the field's Buat Card control; "" when Nudge can't fill it
// (unsupported schema, or a select without any pickable value).
func (f CreateMetaField) FormKind() (kind string, multi bool) {
	kind, multi = f.Schema.FormKind()
	if kind == "" || !NeedsOptions(kind) {
		return kind, multi
	}
	for _, v := range f.AllowedValues {
		if v.Pickable() {
			return kind, multi
		}
	}
	return "", false
}

// BaseFormFieldIDs are filled by the Buat Card form itself (never offered as
// extra fields); the project's Story Point fields are handled too.
var BaseFormFieldIDs = map[string]bool{
	"project": true, "issuetype": true, "summary": true, "description": true, "reporter": true,
	"assignee": true, "parent": true, SprintFieldID: true,
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
			Name    string `json:"name"`
			Subtask bool   `json:"subtask"`
			// HierarchyLevel: -1 subtask, 0 standard (Story/Task/Bug…), 1 Epic.
			HierarchyLevel int    `json:"hierarchyLevel"`
			IconURL        string `json:"iconUrl"`
		} `json:"issuetype"`
		Status struct {
			Name           string `json:"name"`
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
		Subtasks []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
			} `json:"fields"`
		} `json:"subtasks"`
	} `json:"fields"`
}

// IssueBriefPage is a /search/jql answer decoded as briefs.
type IssueBriefPage struct {
	Issues []IssueBrief `json:"issues"`
}

// IssueBriefFields is the fields= of every brief read (single issue or search).
const IssueBriefFields = "summary,project,issuetype,status,subtasks"

type Myself struct {
	AccountID string `json:"accountId"`
}
