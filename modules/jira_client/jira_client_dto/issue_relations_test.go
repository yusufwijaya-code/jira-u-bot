package jira_client_dto

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestIssueRelations_SubtasksThenLinksBothDirections(t *testing.T) {
	raw := `{
	  "key": "OMS-11727",
	  "fields": {
	    "subtasks": [{"key": "OMS-11912"}, {"key": "OMS-11938"}],
	    "issuelinks": [
	      {"type": {"name": "Relates", "inward": "relates to", "outward": "relates to"}, "outwardIssue": {"key": "OMS-11905"}},
	      {"type": {"name": "Blocks", "inward": "is blocked by", "outward": "blocks"}, "inwardIssue": {"key": "CORE-77"}},
	      {"type": {"name": "Relates", "inward": "relates to", "outward": "relates to"}, "outwardIssue": {"key": "OMS-11912"}},
	      {"type": {"name": "Relates"}, "outwardIssue": {"key": "HR-5"}},
	      {"type": {"name": "Cloners"}, "outwardIssue": {"key": "OMS-11727"}},
	      {"type": {"name": "Weird"}, "outwardIssue": {"key": "OMS-1 OR 1=1"}}
	    ]
	  }
	}`
	var issue Issue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatal(err)
	}

	got := issue.Relations()
	want := []Relation{
		{Key: "OMS-11912", Label: "subtask"},
		{Key: "OMS-11938", Label: "subtask"},
		{Key: "OMS-11905", Label: "relates to"},
		{Key: "CORE-77", Label: "is blocked by"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Relations() = %+v, want %+v (dupes, self-links, other projects and malformed keys dropped)", got, want)
	}
}

func TestIsTrackedProjectKey(t *testing.T) {
	for key, want := range map[string]bool{"OMS-1": true, "CORE-10923": true, "HR-1": false, "oms-1": false, "OMS-1)": false, "": false} {
		if got := IsTrackedProjectKey(key); got != want {
			t.Errorf("IsTrackedProjectKey(%q) = %v, want %v", key, got, want)
		}
	}
}
