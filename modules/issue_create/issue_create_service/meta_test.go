package issue_create_service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"nudge/modules/jira_client/jira_client_dto"
)

type schema = jira_client_dto.FieldSchema

func av(id, value string, children ...jira_client_dto.AllowedValue) jira_client_dto.AllowedValue {
	return jira_client_dto.AllowedValue{ID: id, Value: value, Children: children}
}

// omsBugFields mirrors the real OMS Bug createmeta (verified 2026-11-18).
func omsBugFields() []jira_client_dto.CreateMetaField {
	return []jira_client_dto.CreateMetaField{
		{FieldID: "summary", Name: "Summary", Required: true, Schema: schema{Type: "string", System: "summary"}},
		{FieldID: "customfield_10095", Name: "Merge Request", Required: true, Schema: schema{Type: "string", Custom: "com.atlassian.jira.plugin.system.customfieldtypes:textarea"}},
		{FieldID: "customfield_10286", Name: "Service", Required: true, Schema: schema{Type: "option-with-child", Custom: "com.atlassian.jira.plugin.system.customfieldtypes:cascadingselect"},
			AllowedValues: []jira_client_dto.AllowedValue{av("10200", "Order", av("11099", "ESO"), av("11100", "Kiosk")), av("10201", "Other"), {ID: "10202", Value: "Old", Disabled: true}}},
		{FieldID: "customfield_10112", Name: "Related Ticket", Schema: schema{Type: "string", Custom: "com.atlassian.jira.plugin.system.customfieldtypes:textfield"}},
		{FieldID: "customfield_10110", Name: "Dev Notes", Schema: schema{Type: "string", Custom: "com.atlassian.jira.plugin.system.customfieldtypes:textarea"}},
		{FieldID: "customfield_10016", Name: "Story point estimate", Schema: schema{Type: "number"}},
		{FieldID: "customfield_10289", Name: "ISUE POS Version", Schema: schema{Type: "option"}, AllowedValues: []jira_client_dto.AllowedValue{av("10926", "v.2.140.x")}},
		{FieldID: "customfield_10282", Name: "POS Version", Schema: schema{Type: "array", Items: "option"}, AllowedValues: []jira_client_dto.AllowedValue{av("11175", "2.153.x")}},
		{FieldID: "priority", Name: "Priority", Schema: schema{Type: "priority", System: "priority"}, AllowedValues: []jira_client_dto.AllowedValue{{ID: "1", Name: "Highest"}}},
		{FieldID: "versions", Name: "Affects versions", Schema: schema{Type: "array", Items: "version", System: "versions"},
			AllowedValues: []jira_client_dto.AllowedValue{{ID: "10340", Name: "POS v.2.121.0"}, {ID: "10341", Name: "Gone", Archived: true}}},
		{FieldID: "duedate", Name: "Due date", Schema: schema{Type: "date", System: "duedate"}},
		{FieldID: "labels", Name: "Labels", Schema: schema{Type: "array", Items: "string", System: "labels"}},
		{FieldID: "customfield_10284", Name: "Code Review", Schema: schema{Type: "array", Items: "user"}},
		{FieldID: "customfield_10019", Name: "Rank", Schema: schema{Type: "any"}},
		{FieldID: "customfield_10075", Name: "Some time", Schema: schema{Type: "datetime"}},
		{FieldID: "customfield_10999", Name: "Empty select", Schema: schema{Type: "option"}},
	}
}

func fieldByID(tm TypeMeta, id string) (Field, bool) {
	for _, f := range tm.Fields {
		if f.ID == id {
			return f, true
		}
	}
	return Field{}, false
}

func omsRule(extra ...string) jira_client_dto.TrackingRule {
	rule := jira_client_dto.RuleFor("OMS")
	if len(extra) > 0 {
		rule.CreateFields = map[string][]string{"10206": extra}
	}
	return rule
}

func TestMapType_RequiredOnlyWithoutCreateFields(t *testing.T) {
	tm := mapType(omsRule(), jira_client_dto.CreateMetaIssueType{ID: "10206", Name: "Bug"}, omsBugFields())
	if !tm.Supported {
		t.Fatal("OMS Bug must be supported: Merge Request (textarea) and Service (cascade) are fillable")
	}
	var ids []string
	for _, f := range tm.Fields {
		ids = append(ids, f.ID)
	}
	if !reflect.DeepEqual(ids, []string{"customfield_10095", "customfield_10286"}) {
		t.Fatalf("fields = %v, want only the required ones", ids)
	}
	mr, _ := fieldByID(tm, "customfield_10095")
	if mr.Kind != "textarea" || !mr.Required {
		t.Fatalf("merge request = %+v, want a required textarea", mr)
	}
	svc, _ := fieldByID(tm, "customfield_10286")
	if svc.Kind != "cascade" || len(svc.Options) != 2 || len(svc.Options[0].Children) != 2 || svc.Options[1].Children != nil {
		t.Fatalf("service = %+v, want 2 pickable parents, Order with 2 children, Other with none", svc)
	}
}

func TestMapType_CarriesTheTypeIcon(t *testing.T) {
	tm := mapType(omsRule(), jira_client_dto.CreateMetaIssueType{ID: "10206", Name: "Bug", IconURL: "https://jira.example/bug.svg"}, omsBugFields())
	if tm.IconURL != "https://jira.example/bug.svg" {
		t.Fatalf("IconURL = %q, want the createmeta icon", tm.IconURL)
	}
}

func TestMapType_PerKindWithCreateFields(t *testing.T) {
	extra := []string{"customfield_10112", "customfield_10110", "customfield_10289", "customfield_10282", "priority", "versions",
		"duedate", "labels", "customfield_10284", "customfield_10019", "customfield_10075", "customfield_10999", "customfield_10016"}
	tm := mapType(omsRule(extra...), jira_client_dto.CreateMetaIssueType{ID: "10206", Name: "Bug"}, omsBugFields())
	want := map[string]struct {
		kind  string
		multi bool
	}{
		"customfield_10112": {"text", false},
		"customfield_10110": {"textarea", false},
		"customfield_10289": {"select", false},
		"customfield_10282": {"select", true},
		"priority":          {"select", false},
		"versions":          {"select", true},
		"duedate":           {"date", false},
		"labels":            {"labels", true},
	}
	for id, w := range want {
		f, ok := fieldByID(tm, id)
		if !ok || f.Kind != w.kind || f.Multi != w.multi || f.Required {
			t.Errorf("%s = %+v (found %v), want optional %s multi=%v", id, f, ok, w.kind, w.multi)
		}
	}
	for _, id := range []string{"customfield_10284", "customfield_10019", "customfield_10075", "customfield_10999", "customfield_10016"} {
		if _, ok := fieldByID(tm, id); ok {
			t.Errorf("%s must never be offered (unfillable, empty select or the SP field)", id)
		}
	}
	if p, _ := fieldByID(tm, "priority"); p.Options[0].Label != "Highest" {
		t.Errorf("priority options = %+v, want Jira's name as label", p.Options)
	}
	if v, _ := fieldByID(tm, "versions"); len(v.Options) != 1 {
		t.Errorf("versions options = %+v, want archived versions dropped", v.Options)
	}
	if !tm.Supported {
		t.Error("unfillable OPTIONAL fields must not make the type unsupported")
	}
}

func TestMapType_CreateFieldsAreScopedToTheType(t *testing.T) {
	rule := omsRule("duedate")
	tm := mapType(rule, jira_client_dto.CreateMetaIssueType{ID: "10249", Name: "Ticket Issue"}, omsBugFields())
	if _, ok := fieldByID(tm, "duedate"); ok {
		t.Fatal("a field chosen for Bug must not appear on another type")
	}
}

func TestMapType_UnfillableRequiredFieldMakesTypeUnsupported(t *testing.T) {
	fields := []jira_client_dto.CreateMetaField{
		{FieldID: "customfield_1", Name: "QA", Required: true, Schema: schema{Type: "array", Items: "user"}},
	}
	if mapType(omsRule(), jira_client_dto.CreateMetaIssueType{ID: "1"}, fields).Supported {
		t.Fatal("required user picker must make the type unsupported")
	}
	fields[0] = jira_client_dto.CreateMetaField{FieldID: "customfield_2", Name: "Pick", Required: true, Schema: schema{Type: "option"}}
	if mapType(omsRule(), jira_client_dto.CreateMetaIssueType{ID: "1"}, fields).Supported {
		t.Fatal("required select without pickable values must make the type unsupported")
	}
}

func cascadeField() Field {
	return Field{ID: "cf", Name: "Service", Kind: "cascade", Options: []Option{
		{ID: "10200", Label: "Order", Children: []Option{{ID: "11099", Label: "ESO"}}},
		{ID: "10201", Label: "Other"},
	}}
}

func TestEncodeField_PerKind(t *testing.T) {
	sel := Field{Kind: "select", Options: []Option{{ID: "1"}, {ID: "2"}}}
	multi := Field{Kind: "select", Multi: true, Options: []Option{{ID: "1"}, {ID: "2"}}}
	cases := []struct {
		name string
		f    Field
		v    any
		want any
		ok   bool
	}{
		{"text trimmed", Field{Kind: "text"}, "  hi ", "hi", true},
		{"text empty", Field{Kind: "text"}, "   ", "", false},
		{"number", Field{Kind: "number"}, 4.5, 4.5, true},
		{"number missing", Field{Kind: "number"}, nil, 0.0, false},
		{"select", sel, "2", map[string]any{"id": "2"}, true},
		{"select empty", sel, "", nil, false},
		{"multi select dedup", multi, []any{"1", "1", "2", ""}, []map[string]any{{"id": "1"}, {"id": "2"}}, true},
		{"multi select empty", multi, []any{}, []map[string]any{}, false},
		{"cascade parent only", cascadeField(), map[string]any{"id": "10201"}, map[string]any{"id": "10201"}, true},
		{"cascade with child", cascadeField(), map[string]any{"id": "10200", "child": "11099"},
			map[string]any{"id": "10200", "child": map[string]any{"id": "11099"}}, true},
		{"cascade bare parent id", cascadeField(), "10200", map[string]any{"id": "10200"}, true},
		{"cascade empty", cascadeField(), map[string]any{"id": ""}, nil, false},
		{"date", Field{Kind: "date"}, "2026-11-30", "2026-11-30", true},
		{"date empty", Field{Kind: "date"}, "", nil, false},
		{"labels", Field{Kind: "labels", Multi: true}, []any{"be", " fe ", "be", ""}, []string{"be", "fe"}, true},
		{"labels empty", Field{Kind: "labels", Multi: true}, nil, []string{}, false},
	}
	for _, c := range cases {
		got, ok, err := encodeField(c.f, c.v)
		if err != nil || ok != c.ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %#v ok=%v err=%v, want %#v ok=%v", c.name, got, ok, err, c.want, c.ok)
		}
	}
}

func TestEncodeField_TextareaIsADF(t *testing.T) {
	got, ok, err := encodeField(Field{Kind: "textarea"}, "line 1\nline 2")
	doc, isDoc := got.(map[string]any)
	if err != nil || !ok || !isDoc || doc["type"] != "doc" || len(doc["content"].([]any)) != 2 {
		t.Fatalf("got %#v ok=%v err=%v, want an ADF doc with 2 paragraphs", got, ok, err)
	}
	if _, ok, _ := encodeField(Field{Kind: "textarea"}, " \n "); ok {
		t.Fatal("blank textarea must count as empty")
	}
}

func TestEncodeField_RejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		f    Field
		v    any
		msg  string
	}{
		{"date format", Field{Kind: "date"}, "30/11/2026", "YYYY-MM-DD"},
		{"date not a day", Field{Kind: "date"}, "2026-02-30", "YYYY-MM-DD"},
		{"date with time", Field{Kind: "date"}, "2026-11-30T10:00", "YYYY-MM-DD"},
		{"unknown option", Field{Kind: "select", Options: []Option{{ID: "1"}}}, "9", "pilihan"},
		{"unknown multi option", Field{Kind: "select", Multi: true, Options: []Option{{ID: "1"}}}, []any{"1", "9"}, "pilihan"},
		{"unknown cascade parent", cascadeField(), map[string]any{"id": "9"}, "pilihan"},
		{"child of another parent", cascadeField(), map[string]any{"id": "10201", "child": "11099"}, "sub-pilihan"},
		{"label with space", Field{Kind: "labels", Multi: true}, []any{"two words"}, "spasi"},
	}
	for _, c := range cases {
		if _, _, err := encodeField(c.f, c.v); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: err = %v, want one mentioning %q", c.name, err, c.msg)
		}
	}
}

func TestCreateOne_InvalidDynamicValueNamesTheField(t *testing.T) {
	s := newSvc(omsJira(), &fakeAI{})
	_, err := s.CreateOne(context.Background(), "dev@esb.co.id", CreateRequest{BoardID: 28, TypeID: "10139", Summary: "x",
		Fields: map[string]any{"customfield_12001": "mr", "customfield_12002": "9"}})
	var v *ValidationError
	if !errors.As(err, &v) || v.Field != "customfield_12002" || !strings.Contains(v.Message, "Cloud Services") {
		t.Fatalf("err = %#v, want a validation error on customfield_12002", err)
	}
}
