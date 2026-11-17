package tracking_rule

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"nudge/modules/jira_client/jira_client_dto"
)

func TestNormalizeCreateFields(t *testing.T) {
	got, msg := normalizeCreateFields(map[string][]string{" 10206 ": {"duedate", " priority", "duedate"}, "10249": {}})
	if msg != "" || !reflect.DeepEqual(got, map[string][]string{"10206": {"duedate", "priority"}}) {
		t.Fatalf("got %v %q, want trimmed, deduplicated, sorted and empty types dropped", got, msg)
	}
	if got, msg := normalizeCreateFields(nil); got != nil || msg != "" {
		t.Fatalf("nil → %v %q, want nil", got, msg)
	}
	tooMany := make([]string, 51)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("customfield_%d", i)
	}
	cases := map[string]map[string][]string{
		"non-numeric type": {"Bug": {"duedate"}},
		"bad field id":     {"10206": {"customField_1"}},
		"field id spaces":  {"10206": {"due date"}},
		"empty field id":   {"10206": {""}},
		"51 fields":        {"10206": tooMany},
	}
	for name, in := range cases {
		if _, msg := normalizeCreateFields(in); msg == "" {
			t.Errorf("%s: accepted, want a validation message", name)
		}
	}
	if _, msg := normalizeCreateFields(map[string][]string{"10206": tooMany[:50]}); msg != "" {
		t.Errorf("50 fields rejected: %s", msg)
	}
}

func TestCreate_ValidatesAndStoresCreateFields(t *testing.T) {
	h := newHarness(seedRules()...)
	in := validPOS()
	in.CreateFields = map[string][]string{"bug": {"duedate"}}
	_, err := h.svc.Create(ctx, in, "admin@esb.co.id")
	var v *ValidationError
	if !errors.As(err, &v) || v.Fields["createFields"] == "" {
		t.Fatalf("err = %v, want a createFields validation error", err)
	}
	in.CreateFields = map[string][]string{"10206": {"priority", "duedate"}}
	got, err := h.svc.Create(ctx, in, "admin@esb.co.id")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"10206": {"duedate", "priority"}}
	if !reflect.DeepEqual(h.store.rules["POS"].CreateFields, want) || !reflect.DeepEqual(got.CreateFields, want) {
		t.Fatalf("stored %v / view %v, want %v", h.store.rules["POS"].CreateFields, got.CreateFields, want)
	}
	var applied jira_client_dto.TrackingRule
	for _, r := range h.applied[len(h.applied)-1] {
		if r.ProjectKey == "POS" {
			applied = r
		}
	}
	if !applied.ExtraCreateField("10206", "duedate") {
		t.Fatalf("registry rule = %+v, want CreateFields installed (invalidates the Buat Card meta cache)", applied)
	}
}

func TestList_CreateFieldsIsNeverNull(t *testing.T) {
	views, err := newHarness(seedRules()...).svc.List(ctx)
	if err != nil || views[0].CreateFields == nil {
		t.Fatalf("views = %+v, err = %v, want createFields {} for a rule without extra fields", views, err)
	}
}

func omsCreateMeta(h *harness) {
	text := jira_client_dto.FieldSchema{Type: "string"}
	h.jira.createTypes = map[string][]jira_client_dto.CreateMetaIssueType{
		"OMS": {{ID: "10206", Name: "Bug"}, {ID: "10143", Name: "SUB Story", Subtask: true}},
	}
	h.jira.createFields = map[string][]jira_client_dto.CreateMetaField{
		"10206": {
			{FieldID: "summary", Name: "Summary", Required: true, Schema: text},
			{FieldID: "description", Name: "Description", Schema: text},
			{FieldID: "assignee", Name: "Assignee", Schema: jira_client_dto.FieldSchema{Type: "user"}},
			{FieldID: "customfield_10020", Name: "Sprint", Schema: jira_client_dto.FieldSchema{Type: "array", Items: "json"}},
			{FieldID: "customfield_10095", Name: "Merge Request", Required: true, Schema: jira_client_dto.FieldSchema{Type: "string", Custom: "x:textarea"}},
			{FieldID: "customfield_10286", Name: "Service", Required: true, Schema: jira_client_dto.FieldSchema{Type: "option-with-child"},
				AllowedValues: []jira_client_dto.AllowedValue{{ID: "1", Value: "Order"}}},
			{FieldID: "duedate", Name: "Due date", Schema: jira_client_dto.FieldSchema{Type: "date"}},
			{FieldID: "customfield_10284", Name: "Code Review", Schema: jira_client_dto.FieldSchema{Type: "array", Items: "user"}},
			{FieldID: "customfield_10999", Name: "Empty", Schema: jira_client_dto.FieldSchema{Type: "option"}},
		},
	}
}

func TestCreateFields_ListsFillableFieldsPerTypeAndCaches(t *testing.T) {
	h := newHarness(seedRules()...)
	omsCreateMeta(h)
	types, err := h.svc.CreateFields(ctx, " oms ", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 2 || types[0].TypeName != "Bug" || !types[1].Subtask || types[1].Fields == nil {
		t.Fatalf("types = %+v", types)
	}
	want := []CreateField{
		{ID: "customfield_10095", Name: "Merge Request", Kind: "textarea", Required: true},
		{ID: "customfield_10286", Name: "Service", Kind: "cascade", Required: true},
		{ID: "duedate", Name: "Due date", Kind: "date"},
	}
	if !reflect.DeepEqual(types[0].Fields, want) {
		t.Fatalf("bug fields = %+v, want base-form, unfillable and empty-select fields left out", types[0].Fields)
	}
	h.svc.CreateFields(ctx, "OMS", false)
	if h.jira.createCalls != 1 {
		t.Errorf("createmeta read %d times, want 1 (cached)", h.jira.createCalls)
	}
	h.svc.CreateFields(ctx, "OMS", true)
	if h.jira.createCalls != 2 {
		t.Errorf("fresh read %d times, want 2 (fresh bypasses the cache)", h.jira.createCalls)
	}
}

func TestCreateFields_Errors(t *testing.T) {
	h := newHarness(seedRules()...)
	omsCreateMeta(h)
	if _, err := h.svc.CreateFields(ctx, "x", false); !errors.Is(err, ErrInvalidProjectKey) {
		t.Errorf("bad key err = %v", err)
	}
	if _, err := h.svc.CreateFields(ctx, "NOPE", false); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("unknown project err = %v", err)
	}
	h.jira.err = errors.New("boom")
	if _, err := h.svc.CreateFields(ctx, "OMS", true); !errors.Is(err, ErrJiraUnavailable) || strings.Contains(err.Error(), "boom") {
		t.Errorf("jira down err = %v, want ErrJiraUnavailable", err)
	}
}
