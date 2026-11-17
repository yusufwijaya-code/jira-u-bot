package jira_client_dto

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DTOs for moves that need fields filled first ("move-fields"): the
// workflow rules (team-managed projects keep required fields as transition
// validators), transition screens (company-managed projects), editmeta and
// the issue's current values.

// ScreenTransition is GET /issue/{key}/transitions?expand=transitions.fields:
// a transition with its screen fields (company-managed only).
type ScreenTransition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"to"`
	Fields map[string]CreateMetaField `json:"fields"`
}

type ScreenTransitionsResponse struct {
	Transitions []ScreenTransition `json:"transitions"`
}

// WorkflowSearchRequest is the body of POST /rest/api/3/workflows.
type WorkflowSearchRequest struct {
	ProjectAndIssueTypes []ProjectIssueType `json:"projectAndIssueTypes"`
}

type ProjectIssueType struct {
	ProjectID   string `json:"projectId"`
	IssueTypeID string `json:"issueTypeId"`
}

type WorkflowsResponse struct {
	Workflows []struct {
		Transitions []WorkflowTransition `json:"transitions"`
	} `json:"workflows"`
}

type WorkflowTransition struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	ToStatusReference string         `json:"toStatusReference"`
	Validators        []WorkflowRule `json:"validators"`
}

type WorkflowRule struct {
	RuleKey    string         `json:"ruleKey"`
	Parameters map[string]any `json:"parameters"`
}

// Param is a rule parameter as a string ("" when absent).
func (r WorkflowRule) Param(name string) string {
	switch v := r.Parameters[name].(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// RequiredFields: the field ids a "system:validate-field-value" validator
// insists on (fieldRequired → comma-separated fieldsRequired, fieldHasSingleValue
// → fieldKey) and its error message. Any other rule yields nothing.
func (r WorkflowRule) RequiredFields() (ids []string, message string) {
	if r.RuleKey != "system:validate-field-value" {
		return nil, ""
	}
	switch r.Param("ruleType") {
	case "fieldRequired":
		for _, id := range strings.Split(r.Param("fieldsRequired"), ",") {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
	case "fieldHasSingleValue":
		if id := strings.TrimSpace(r.Param("fieldKey")); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, strings.TrimSpace(r.Param("errorMessage"))
}

// EditMetaResponse is GET /issue/{key}/editmeta: fields keyed by id.
type EditMetaResponse struct {
	Fields map[string]CreateMetaField `json:"fields"`
}

// IssueFieldValues is GET /issue/{key}?fields=project,issuetype,<ids>: the
// ids needed for the workflow lookup plus each asked field's raw value.
type IssueFieldValues struct {
	Key         string
	ProjectID   string
	ProjectKey  string
	IssueTypeID string
	Values      map[string]json.RawMessage
}

func (v *IssueFieldValues) UnmarshalJSON(b []byte) error {
	var raw struct {
		Key    string                     `json:"key"`
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	var ref struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	v.Key, v.Values = raw.Key, raw.Fields
	if json.Unmarshal(raw.Fields["project"], &ref) == nil {
		v.ProjectID, v.ProjectKey = ref.ID, ref.Key
	}
	ref.ID = ""
	if json.Unmarshal(raw.Fields["issuetype"], &ref) == nil {
		v.IssueTypeID = ref.ID
	}
	if v.Values == nil {
		v.Values = map[string]json.RawMessage{}
	}
	return nil
}

// TransitionWithFieldsRequest is POST /issue/{key}/transitions with screen fields.
type TransitionWithFieldsRequest struct {
	Transition TransitionID   `json:"transition"`
	Fields     map[string]any `json:"fields,omitempty"`
}

// EditIssueRequest is PUT /issue/{key}.
type EditIssueRequest struct {
	Fields map[string]any `json:"fields"`
}
