package issue_create_service

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"nudge/modules/jira_client/jira_client_dto"
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
	tm := TypeMeta{ID: t.ID, Name: t.Name, Subtask: t.Subtask, Supported: true, Fields: []Field{}}
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

func toField(f jira_client_dto.CreateMetaField) (Field, bool) {
	kind, multi := f.FormKind()
	if kind == "" {
		return Field{}, false
	}
	field := Field{ID: f.ID(), Name: f.Name, Kind: kind, Multi: multi, Required: f.Required}
	if jira_client_dto.NeedsOptions(kind) {
		field.Options = options(f.AllowedValues, kind == jira_client_dto.FieldKindCascade)
	}
	return field, true
}

func options(values []jira_client_dto.AllowedValue, withChildren bool) []Option {
	out := []Option{}
	for _, v := range values {
		if !v.Pickable() {
			continue
		}
		o := Option{ID: v.ID, Label: v.Label()}
		if withChildren {
			o.Children = options(v.Children, false)
			if len(o.Children) == 0 {
				o.Children = nil
			}
		}
		out = append(out, o)
	}
	return out
}

func (m Meta) typeByID(id string) (TypeMeta, bool) {
	for _, t := range m.Types {
		if t.ID == id {
			return t, true
		}
	}
	return TypeMeta{}, false
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// encodeField turns a form value into Jira's create shape; ok=false when
// empty, err when the value can't be sent (unknown option, bad date…).
func encodeField(f Field, v any) (value any, ok bool, err error) {
	switch f.Kind {
	case jira_client_dto.FieldKindText:
		s := text(v)
		return s, s != "", nil
	case jira_client_dto.FieldKindTextarea:
		// Multi-line custom fields take ADF on API v3.
		s := text(v)
		return adf(s), s != "", nil
	case jira_client_dto.FieldKindNumber:
		n, isNum := v.(float64)
		if isNum && n != n {
			return nil, false, errors.New("angka tidak valid")
		}
		return n, isNum, nil
	case jira_client_dto.FieldKindDate:
		s := text(v)
		if s == "" {
			return nil, false, nil
		}
		if _, perr := time.Parse("2006-01-02", s); perr != nil || !dateRe.MatchString(s) {
			return nil, false, errors.New("tanggal harus YYYY-MM-DD")
		}
		return s, true, nil
	case jira_client_dto.FieldKindLabels:
		out := []string{}
		for _, x := range list(v) {
			l := text(x)
			if l == "" || slices.Contains(out, l) {
				continue
			}
			if strings.ContainsAny(l, " \t\n") {
				return nil, false, fmt.Errorf("label %q tidak boleh berisi spasi", l)
			}
			out = append(out, l)
		}
		return out, len(out) > 0, nil
	case jira_client_dto.FieldKindSelect:
		if f.Multi {
			out := []map[string]any{}
			seen := map[string]bool{}
			for _, x := range list(v) {
				id := text(x)
				if id == "" || seen[id] {
					continue
				}
				if _, found := optionByID(f.Options, id); !found {
					return nil, false, errors.New("pilihan tidak valid")
				}
				seen[id] = true
				out = append(out, map[string]any{"id": id})
			}
			return out, len(out) > 0, nil
		}
		id := text(v)
		if id == "" {
			return nil, false, nil
		}
		if _, found := optionByID(f.Options, id); !found {
			return nil, false, errors.New("pilihan tidak valid")
		}
		return map[string]any{"id": id}, true, nil
	case jira_client_dto.FieldKindCascade:
		// {"id": parent, "child": child} from the form; a bare parent id is accepted too.
		var parentID, childID string
		switch c := v.(type) {
		case string:
			parentID = strings.TrimSpace(c)
		case map[string]any:
			parentID, childID = text(c["id"]), text(c["child"])
		}
		if parentID == "" {
			return nil, false, nil
		}
		parent, found := optionByID(f.Options, parentID)
		if !found {
			return nil, false, errors.New("pilihan tidak valid")
		}
		out := map[string]any{"id": parentID}
		if childID != "" {
			if _, found := optionByID(parent.Children, childID); !found {
				return nil, false, errors.New("sub-pilihan tidak valid")
			}
			out["child"] = map[string]any{"id": childID}
		}
		return out, true, nil
	}
	return nil, false, nil
}

func text(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}

func optionByID(opts []Option, id string) (Option, bool) {
	for _, o := range opts {
		if o.ID == id {
			return o, true
		}
	}
	return Option{}, false
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
