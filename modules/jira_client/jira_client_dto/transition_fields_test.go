package jira_client_dto

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestWorkflowRule_RequiredFields(t *testing.T) {
	cases := []struct {
		name    string
		rule    WorkflowRule
		ids     []string
		message string
	}{
		{"fieldRequired list", WorkflowRule{RuleKey: "system:validate-field-value", Parameters: map[string]any{
			"ruleType": "fieldRequired", "fieldsRequired": "customfield_10095, customfield_2,", "errorMessage": "Fill in the Merge Request Fields Before Code Review"}},
			[]string{"customfield_10095", "customfield_2"}, "Fill in the Merge Request Fields Before Code Review"},
		{"fieldHasSingleValue", WorkflowRule{RuleKey: "system:validate-field-value", Parameters: map[string]any{
			"ruleType": "fieldHasSingleValue", "fieldKey": "customfield_10095", "excludeSubtasks": "false"}},
			[]string{"customfield_10095"}, ""},
		{"other rule type", WorkflowRule{RuleKey: "system:validate-field-value", Parameters: map[string]any{"ruleType": "fieldChanged", "fieldKey": "x"}}, nil, ""},
		{"other rule key", WorkflowRule{RuleKey: "system:check-permission-validator", Parameters: map[string]any{"permissionKey": "ADMINISTER"}}, nil, ""},
	}
	for _, c := range cases {
		ids, msg := c.rule.RequiredFields()
		if !reflect.DeepEqual(ids, c.ids) || msg != c.message {
			t.Errorf("%s: got %v %q, want %v %q", c.name, ids, msg, c.ids, c.message)
		}
	}
}

func TestIssueFieldValues_Unmarshal(t *testing.T) {
	var v IssueFieldValues
	body := `{"key":"OMS-1","fields":{"project":{"id":"10039","key":"OMS"},"issuetype":{"id":"10143","name":"SUB Story"},"customfield_10095":null}}`
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatal(err)
	}
	if v.Key != "OMS-1" || v.ProjectID != "10039" || v.ProjectKey != "OMS" || v.IssueTypeID != "10143" {
		t.Errorf("got %+v", v)
	}
	if string(v.Values["customfield_10095"]) != "null" {
		t.Errorf("raw value = %s", v.Values["customfield_10095"])
	}
}
