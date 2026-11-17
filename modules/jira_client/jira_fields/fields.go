// Package jira_fields is the one place that turns Jira field metadata
// (createmeta, editmeta, transition screens) into the form fields Nudge can
// fill, encodes a form value into Jira's write shape, and tells whether a
// value read back from Jira is empty. Shared by Buat Card (issue_create) and
// moves that need fields filled first (user_dashboard, "move-fields").
package jira_fields

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"nudge/modules/jira_client/jira_client_dto"
)

// KindUnsupported marks a field Nudge can't fill (user picker, attachment…):
// the move dialog shows it with an "Isi di Jira" link instead of a control.
const KindUnsupported = "unsupported"

type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Children: second-level options of a "cascade" field.
	Children []Option `json:"children,omitempty"`
}

type Field struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Multi    bool     `json:"multi"`
	Required bool     `json:"required"`
	Options  []Option `json:"options,omitempty"`
}

// FromMeta turns one metadata field into a form Field; ok=false when Nudge
// can't fill it (unsupported schema, or a select without pickable values).
func FromMeta(f jira_client_dto.CreateMetaField) (Field, bool) {
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

// FromMetaOrUnsupported is FromMeta, falling back to an "unsupported" Field
// that still carries the field's id and name.
func FromMetaOrUnsupported(f jira_client_dto.CreateMetaField) Field {
	if field, ok := FromMeta(f); ok {
		return field
	}
	name := f.Name
	if name == "" {
		name = f.ID()
	}
	return Field{ID: f.ID(), Name: name, Kind: KindUnsupported, Required: f.Required}
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

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Encode turns a form value into Jira's write shape (create, edit and
// transition bodies all take the same one); ok=false when empty, err when
// the value can't be sent (unknown option, bad date…).
func Encode(f Field, v any) (value any, ok bool, err error) {
	switch f.Kind {
	case jira_client_dto.FieldKindText:
		s := text(v)
		return s, s != "", nil
	case jira_client_dto.FieldKindTextarea:
		// Multi-line custom fields take ADF on API v3.
		s := text(v)
		return ADF(s), s != "", nil
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

// ADF wraps plain text as Atlassian Document Format, one paragraph per non-empty line.
func ADF(text string) map[string]any {
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

// IsEmpty reports whether a field value read from Jira (GET issue) counts
// as not filled: absent/null, blank text, an ADF document without any text
// or inline content, an empty array, or an object without id/value/name/key.
func IsEmpty(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return true
	}
	return emptyValue(v)
}

func emptyValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []any:
		for _, item := range x {
			if !emptyValue(item) {
				return false
			}
		}
		return true
	case map[string]any:
		if t, _ := x["type"].(string); t == "doc" {
			return adfEmpty(x)
		}
		for _, k := range []string{"id", "value", "name", "key", "accountId"} {
			if s, ok := x[k].(string); ok && strings.TrimSpace(s) != "" {
				return false
			}
		}
		return len(x) == 0
	}
	// numbers, booleans
	return false
}

// adfEmpty: an ADF node is empty when it holds no non-blank text and no
// inline/block content other than paragraphs and line breaks (a mention,
// link card, media… counts as filled).
func adfEmpty(node map[string]any) bool {
	switch t, _ := node["type"].(string); t {
	case "text":
		s, _ := node["text"].(string)
		return strings.TrimSpace(s) == ""
	case "doc", "paragraph", "hardBreak", "":
	default:
		return false
	}
	children, _ := node["content"].([]any)
	for _, c := range children {
		if m, ok := c.(map[string]any); ok && !adfEmpty(m) {
			return false
		}
	}
	return true
}
