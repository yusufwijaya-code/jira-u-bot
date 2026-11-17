package move_fields

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_client_service"
	"nudge/modules/jira_client/jira_fields"
)

var cred = &jira_client_dto.Credential{Email: "dev@esb.co.id", APIToken: "t"}

const mrField = "customfield_10095"

type fakeJira struct {
	workflows    []jira_client_dto.WorkflowTransition
	workflowErr  error
	workflowHits int
	screens      []jira_client_dto.ScreenTransition
	edit         map[string]jira_client_dto.CreateMetaField
	values       map[string]json.RawMessage
	editErr      error
	transErr     error
	calls        []string
	edited       map[string]any
	transFields  map[string]any
}

func (f *fakeJira) WorkflowTransitions(ctx context.Context, projectID, typeID string) ([]jira_client_dto.WorkflowTransition, error) {
	f.workflowHits++
	return f.workflows, f.workflowErr
}
func (f *fakeJira) ScreenTransitionsAs(ctx context.Context, c *jira_client_dto.Credential, key string) ([]jira_client_dto.ScreenTransition, error) {
	return f.screens, nil
}
func (f *fakeJira) EditMetaAs(ctx context.Context, c *jira_client_dto.Credential, key string) (map[string]jira_client_dto.CreateMetaField, error) {
	return f.edit, nil
}
func (f *fakeJira) FieldValuesAs(ctx context.Context, c *jira_client_dto.Credential, key string, ids []string) (jira_client_dto.IssueFieldValues, error) {
	v := jira_client_dto.IssueFieldValues{Key: key, ProjectID: "10039", IssueTypeID: "10143", Values: map[string]json.RawMessage{}}
	for _, id := range ids {
		v.Values[id] = f.values[id]
	}
	return v, nil
}
func (f *fakeJira) EditIssueAs(ctx context.Context, c *jira_client_dto.Credential, key string, fields map[string]any) error {
	f.calls = append(f.calls, "edit")
	f.edited = fields
	return f.editErr
}
func (f *fakeJira) TransitionWithFieldsAs(ctx context.Context, c *jira_client_dto.Credential, key, tid string, fields map[string]any) error {
	f.calls = append(f.calls, "transition:"+tid)
	f.transFields = fields
	return f.transErr
}

var _ jira_client_service.TransitionFieldsClient = (*fakeJira)(nil)

func mrMeta() jira_client_dto.CreateMetaField {
	return jira_client_dto.CreateMetaField{FieldID: mrField, Name: "Merge Request",
		Schema: jira_client_dto.FieldSchema{Type: "string", Custom: "com.atlassian.jira.plugin.system.customfieldtypes:textarea"}}
}

// omsJira: OMS "SUB Story" → Code Review (transition 9) requires Merge Request, empty on the ticket.
func omsJira() *fakeJira {
	return &fakeJira{
		workflows: []jira_client_dto.WorkflowTransition{{ID: "9", Name: "Merge Request to Snapshot Branch", ToStatusReference: "10213",
			Validators: []jira_client_dto.WorkflowRule{
				{RuleKey: "system:validate-field-value", Parameters: map[string]any{"ruleType": "fieldHasSingleValue", "fieldKey": mrField}},
				{RuleKey: "system:validate-field-value", Parameters: map[string]any{"ruleType": "fieldRequired", "fieldsRequired": mrField, "errorMessage": "Fill in the Merge Request Fields Before Code Review"}},
			}}},
		edit:   map[string]jira_client_dto.CreateMetaField{mrField: mrMeta()},
		values: map[string]json.RawMessage{mrField: json.RawMessage(`null`)},
	}
}

var toReview = Transition{ID: "9", Name: "Merge Request to Snapshot Branch", To: "Code Review", ToID: "10213"}

func TestCheck_ValidatorFieldEmpty_ReturnsFieldsRequiredDeduped(t *testing.T) {
	j := omsJira()
	err := New(j).Check(context.Background(), cred, "OMS-1", toReview)
	var fr *FieldsRequiredError
	if !errors.As(err, &fr) {
		t.Fatalf("err = %v, want FieldsRequiredError", err)
	}
	r := fr.Required
	if len(r.Fields) != 1 || r.Fields[0].ID != mrField || r.Fields[0].Kind != "textarea" || !r.Fields[0].Required {
		t.Errorf("fields = %+v (fieldRequired + fieldHasSingleValue must dedupe)", r.Fields)
	}
	if r.Message != "Fill in the Merge Request Fields Before Code Review" || r.Transition.ID != "9" || r.IssueKey != "OMS-1" {
		t.Errorf("required = %+v", r)
	}
	if fr.FieldNames() != "Merge Request" {
		t.Errorf("FieldNames = %q", fr.FieldNames())
	}
}

func TestCheck_FilledADF_Passes(t *testing.T) {
	j := omsJira()
	j.values[mrField] = json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"MR !1"}]}]}`)
	if err := New(j).Check(context.Background(), cred, "OMS-1", toReview); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}

func TestCheck_OtherTransitionAndRulesCached(t *testing.T) {
	j := omsJira()
	s := New(j)
	if err := s.Check(context.Background(), cred, "OMS-1", Transition{ID: "2", To: "Done", ToID: "10157"}); err != nil {
		t.Errorf("err = %v", err)
	}
	s.Check(context.Background(), cred, "OMS-2", toReview)
	if j.workflowHits != 1 {
		t.Errorf("workflow fetched %d times, want 1 (cached per project/type)", j.workflowHits)
	}
}

func TestCheck_ScreenRequiredFieldAndUnsupported(t *testing.T) {
	// Company-managed: the transition screen marks fields required.
	j := &fakeJira{workflowErr: errors.New("403"), values: map[string]json.RawMessage{}}
	j.screens = []jira_client_dto.ScreenTransition{{ID: "31", Name: "Resolve", Fields: map[string]jira_client_dto.CreateMetaField{
		"resolution": {FieldID: "resolution", Name: "Resolution", Required: true, Schema: jira_client_dto.FieldSchema{Type: "resolution"}},
		"customfield_1": {FieldID: "customfield_1", Name: "Root cause", Required: true, Schema: jira_client_dto.FieldSchema{Type: "string"}},
		"comment":       {FieldID: "comment", Name: "Comment", Required: false},
	}}}
	err := New(j).Check(context.Background(), cred, "CORE-1", Transition{ID: "31", To: "Done"})
	var fr *FieldsRequiredError
	if !errors.As(err, &fr) || len(fr.Required.Fields) != 2 {
		t.Fatalf("err = %v", err)
	}
	kinds := map[string]string{}
	for _, f := range fr.Required.Fields {
		kinds[f.ID] = f.Kind
	}
	if kinds["resolution"] != jira_fields.KindUnsupported || kinds["customfield_1"] != "text" {
		t.Errorf("kinds = %v", kinds)
	}
	if !strings.Contains(fr.Required.Message, "Done") {
		t.Errorf("default message = %q", fr.Required.Message)
	}
}

func TestFromRejection_InvalidatesCacheAndUsesJiraFields(t *testing.T) {
	j := &fakeJira{edit: map[string]jira_client_dto.CreateMetaField{"customfield_7": {FieldID: "customfield_7", Name: "Release note", Schema: jira_client_dto.FieldSchema{Type: "string"}}}}
	s := New(j)
	tr := Transition{ID: "5", To: "Code Review"}
	s.Check(context.Background(), cred, "CORE-1", tr) // caches "no rules"
	err := s.FromRejection(context.Background(), cred, "CORE-1", tr, &jira_client_service.RejectedError{Fields: map[string]string{"customfield_7": "Release note is required."}})
	var fr *FieldsRequiredError
	if !errors.As(err, &fr) || len(fr.Required.Fields) != 1 || fr.Required.Fields[0].Name != "Release note" {
		t.Fatalf("err = %v", err)
	}
	if fr.Required.Message != "Release note is required." {
		t.Errorf("message = %q", fr.Required.Message)
	}
	if j.workflowHits != 2 {
		t.Errorf("workflow fetched %d times, want 2 (cache invalidated)", j.workflowHits)
	}
}

func TestFromRejection_MessageOnly(t *testing.T) {
	err := New(&fakeJira{}).FromRejection(context.Background(), cred, "CORE-1", Transition{ID: "5", To: "Done"},
		&jira_client_service.RejectedError{Messages: []string{"Sub-tasks must be done first"}})
	var fr *FieldsRequiredError
	if !errors.As(err, &fr) || len(fr.Required.Fields) != 0 || fr.Required.Message != "Sub-tasks must be done first" {
		t.Fatalf("err = %v", err)
	}
}

func TestApply_EditsThenTransitions(t *testing.T) {
	j := omsJira()
	err := New(j).Apply(context.Background(), cred, "OMS-1", toReview, map[string]any{mrField: "https://gitlab/mr/1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(j.calls, ",") != "edit,transition:9" {
		t.Errorf("calls = %v, want edit then transition", j.calls)
	}
	doc, _ := j.edited[mrField].(map[string]any)
	if doc["type"] != "doc" {
		t.Errorf("textarea not sent as ADF: %v", j.edited)
	}
	if len(j.transFields) != 0 {
		t.Errorf("validator-only field sent in transition body: %v", j.transFields)
	}
}

func TestApply_ScreenFieldsGoWithTransition(t *testing.T) {
	j := &fakeJira{workflowErr: errors.New("403"), values: map[string]json.RawMessage{}}
	j.screens = []jira_client_dto.ScreenTransition{{ID: "31", Fields: map[string]jira_client_dto.CreateMetaField{
		"customfield_1": {FieldID: "customfield_1", Name: "Root cause", Required: true, Schema: jira_client_dto.FieldSchema{Type: "string"}},
	}}}
	if err := New(j).Apply(context.Background(), cred, "CORE-1", Transition{ID: "31", To: "Done"}, map[string]any{"customfield_1": "typo"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(j.calls, ",") != "transition:31" || j.transFields["customfield_1"] != "typo" {
		t.Errorf("calls = %v, fields = %v", j.calls, j.transFields)
	}
}

func TestApply_StillEmptyOrInvalid(t *testing.T) {
	j := omsJira()
	err := New(j).Apply(context.Background(), cred, "OMS-1", toReview, map[string]any{mrField: "  "})
	var fr *FieldsRequiredError
	if !errors.As(err, &fr) || len(j.calls) != 0 {
		t.Errorf("blank value: err = %v, calls = %v", err, j.calls)
	}
	j.edit["customfield_9"] = jira_client_dto.CreateMetaField{FieldID: "customfield_9", Name: "Due", Schema: jira_client_dto.FieldSchema{Type: "date"}}
	err = New(j).Apply(context.Background(), cred, "OMS-1", toReview, map[string]any{mrField: "x", "customfield_9": "31/12"})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Fields["customfield_9"] == "" {
		t.Errorf("bad date: err = %v", err)
	}
}

func TestApply_TransitionRejectedBecomesFieldsRequired(t *testing.T) {
	j := omsJira()
	j.transErr = &jira_client_service.RejectedError{Messages: []string{"Nope"}}
	err := New(j).Apply(context.Background(), cred, "OMS-1", toReview, map[string]any{mrField: "x"})
	var fr *FieldsRequiredError
	if !errors.As(err, &fr) || fr.Required.Message != "Nope" {
		t.Errorf("err = %v", err)
	}
}
