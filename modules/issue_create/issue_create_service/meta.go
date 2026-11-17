package issue_create_service

import (
	"strings"

	"nudge/modules/jira_client/jira_client_dto"
)

// baseHandled: fields the form handles itself — never rendered as dynamic
// fields. Every Story Point field of the project's rule is handled too.
var baseHandled = map[string]bool{
	"project": true, "issuetype": true, "summary": true, "description": true, "reporter": true,
	"assignee": true, "parent": true, jira_client_dto.SprintFieldID: true,
}

func handledFor(rule jira_client_dto.TrackingRule, id string) bool {
	if baseHandled[id] || id == rule.StoryPointWriteField || id == rule.StoryPointWriteFieldSubtask {
		return true
	}
	for _, sp := range rule.StoryPointFields {
		if sp == id {
			return true
		}
	}
	return false
}

// mapType turns one createmeta issue type into the form's TypeMeta. rule is
// the project's tracking rule (Tech Spec "Aturan Board", AC5.3): the SP
// input writes StoryPointWriteFieldSubtask for a subtask type and
// StoryPointWriteField otherwise — only when that field is on the type's
// createmeta; otherwise the SP input is hidden (AC5.3b).
func mapType(rule jira_client_dto.TrackingRule, t jira_client_dto.CreateMetaIssueType, fields []jira_client_dto.CreateMetaField) TypeMeta {
	tm := TypeMeta{ID: t.ID, Name: t.Name, Subtask: t.Subtask, Supported: true, Fields: []Field{}}
	ids := map[string]bool{}
	for _, f := range fields {
		ids[f.ID()] = true
	}
	write := rule.StoryPointWriteField
	if t.Subtask {
		write = rule.StoryPointWriteFieldSubtask
	}
	if write != "" && ids[write] {
		tm.StoryPointField = write
	}
	tm.ParentAllowed = ids["parent"]
	for _, f := range fields {
		if handledFor(rule, f.ID()) || !f.Required {
			continue
		}
		kind, multi := kindOf(f.Schema)
		if kind == "" {
			tm.Supported = false
			continue
		}
		field := Field{ID: f.ID(), Name: f.Name, Kind: kind, Multi: multi, Required: true}
		for _, v := range f.AllowedValues {
			field.Options = append(field.Options, Option{ID: v.ID, Label: v.Label()})
		}
		tm.Fields = append(tm.Fields, field)
	}
	return tm
}

func kindOf(s jira_client_dto.FieldSchema) (kind string, multi bool) {
	switch s.Type {
	case "string":
		return "text", false
	case "number":
		return "number", false
	case "option":
		return "select", false
	case "array":
		if s.Items == "option" {
			return "select", true
		}
	}
	return "", false
}

func (m Meta) typeByID(id string) (TypeMeta, bool) {
	for _, t := range m.Types {
		if t.ID == id {
			return t, true
		}
	}
	return TypeMeta{}, false
}

// encodeField turns a form value into Jira's create shape; ok=false when empty.
func encodeField(f Field, v any) (any, bool) {
	switch f.Kind {
	case "text":
		s, _ := v.(string)
		s = strings.TrimSpace(s)
		return s, s != ""
	case "number":
		n, ok := v.(float64)
		return n, ok
	case "select":
		if f.Multi {
			raw, _ := v.([]any)
			out := []map[string]any{}
			for _, x := range raw {
				if id, _ := x.(string); id != "" {
					out = append(out, map[string]any{"id": id})
				}
			}
			return out, len(out) > 0
		}
		id, _ := v.(string)
		return map[string]any{"id": id}, id != ""
	}
	return nil, false
}

// adf wraps plain text as Atlassian Document Format, one paragraph per non-empty line.
func adf(text string) map[string]any {
	content := []any{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		content = append(content, map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": line}}})
	}
	return map[string]any{"type": "doc", "version": 1, "content": content}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
