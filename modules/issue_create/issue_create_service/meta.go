package issue_create_service

import (
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_fields"
)

func handledFor(rule jira_client_dto.TrackingRule, id string) bool {
	if jira_client_dto.BaseFormFieldIDs[id] || id == rule.StoryPointWriteField || id == rule.StoryPointWriteFieldSubtask {
		return true
	}
	for _, sp := range rule.StoryPointFields {
		if sp == id {
			return true
		}
	}
	return false
}

// storyPointFieldFor is preferred when the type has it, else the first of the
// rule's read fields the type has; "" when the type has none.
func storyPointFieldFor(preferred string, readOrder []string, onType map[string]bool) string {
	if preferred != "" && onType[preferred] {
		return preferred
	}
	for _, id := range readOrder {
		if onType[id] {
			return id
		}
	}
	return ""
}

// mapType turns one createmeta issue type into the form's TypeMeta. rule is
// the project's tracking rule (Tech Spec "Aturan Board", AC5.3): the SP
// input writes StoryPointWriteFieldSubtask for a subtask type and
// StoryPointWriteField otherwise. When that field isn't on the type's
// createmeta, the next of the rule's Story Point fields that is (in read
// order) is used — user decision 2026-11-17: CORE cards use "Story Point
// Estimate" when the type has it, else "Story Points". No SP field at all on
// the type → the SP input is hidden (AC5.3b).
//
// Dynamic fields: every required field, plus the optional ones the rule's
// CreateFields picks for this type. A required field Nudge can't fill makes
// the type unsupported; an optional one is never offered.
func mapType(rule jira_client_dto.TrackingRule, t jira_client_dto.CreateMetaIssueType, fields []jira_client_dto.CreateMetaField) TypeMeta {
	tm := TypeMeta{ID: t.ID, Name: t.Name, IconURL: t.IconURL, Subtask: t.Subtask, Supported: true, Fields: []Field{}}
	ids := map[string]bool{}
	for _, f := range fields {
		ids[f.ID()] = true
	}
	write := rule.StoryPointWriteField
	if t.Subtask {
		write = rule.StoryPointWriteFieldSubtask
	}
	tm.StoryPointField = storyPointFieldFor(write, rule.StoryPointFields, ids)
	tm.ParentAllowed = ids["parent"]
	for _, f := range fields {
		if handledFor(rule, f.ID()) || (!f.Required && !rule.ExtraCreateField(t.ID, f.ID())) {
			continue
		}
		field, ok := toField(f)
		if !ok {
			if f.Required {
				tm.Supported = false
			}
			continue
		}
		tm.Fields = append(tm.Fields, field)
	}
	return tm
}

// toField / encodeField / adf: the shared jira_fields helpers (also used by
// moves that need fields filled first).
func toField(f jira_client_dto.CreateMetaField) (Field, bool) { return jira_fields.FromMeta(f) }

func (m Meta) typeByID(id string) (TypeMeta, bool) {
	for _, t := range m.Types {
		if t.ID == id {
			return t, true
		}
	}
	return TypeMeta{}, false
}

func encodeField(f Field, v any) (value any, ok bool, err error) { return jira_fields.Encode(f, v) }

func adf(text string) map[string]any { return jira_fields.ADF(text) }

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
