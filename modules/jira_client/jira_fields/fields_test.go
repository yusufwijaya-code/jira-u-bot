package jira_fields

import (
	"encoding/json"
	"testing"

	"nudge/modules/jira_client/jira_client_dto"
)

func TestIsEmpty_PerValueKind(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"absent", ``, true},
		{"null", `null`, true},
		{"blank string", `"  "`, true},
		{"string", `"MR-1"`, false},
		{"empty ADF", `{"type":"doc","version":1,"content":[]}`, true},
		{"ADF blank paragraph", `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":" "},{"type":"hardBreak"}]}]}`, true},
		{"ADF text", `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"https://gitlab/mr/1"}]}]}`, false},
		{"ADF inline card only", `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"inlineCard","attrs":{"url":"https://x"}}]}]}`, false},
		{"option", `{"self":"x","id":"10001","value":"High"}`, false},
		{"empty object", `{}`, true},
		{"empty array", `[]`, true},
		{"array of options", `[{"id":"1"}]`, false},
		{"array of blank strings", `[""]`, true},
		{"number zero", `0`, false},
		{"user", `{"accountId":"abc"}`, false},
	}
	for _, c := range cases {
		if got := IsEmpty(json.RawMessage(c.raw)); got != c.want {
			t.Errorf("%s: IsEmpty(%s) = %v, want %v", c.name, c.raw, got, c.want)
		}
	}
}

func TestFromMetaOrUnsupported(t *testing.T) {
	textarea := jira_client_dto.CreateMetaField{FieldID: "customfield_10095", Name: "Merge Request",
		Schema: jira_client_dto.FieldSchema{Type: "string", Custom: "com.atlassian.jira.plugin.system.customfieldtypes:textarea"}}
	if f := FromMetaOrUnsupported(textarea); f.Kind != jira_client_dto.FieldKindTextarea || f.Name != "Merge Request" {
		t.Errorf("textarea field = %+v", f)
	}
	user := jira_client_dto.CreateMetaField{FieldID: "customfield_1", Name: "Reviewer", Schema: jira_client_dto.FieldSchema{Type: "user"}}
	if f := FromMetaOrUnsupported(user); f.Kind != KindUnsupported || f.ID != "customfield_1" || f.Name != "Reviewer" {
		t.Errorf("user field = %+v, want unsupported", f)
	}
}
