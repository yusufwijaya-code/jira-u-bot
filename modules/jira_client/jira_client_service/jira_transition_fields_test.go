package jira_client_service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkflowTransitions_UsesOperatorCredentialAndParsesValidators(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/workflows" || r.URL.Query().Get("expand") != "values.transitions" {
			t.Errorf("request = %s %s", r.Method, r.URL)
		}
		if user, _, _ := r.BasicAuth(); user != "bot@esb.co.id" {
			t.Errorf("auth user = %q, want the operator", user)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"projectId":"10039"`) || !strings.Contains(string(body), `"issueTypeId":"10143"`) {
			t.Errorf("body = %s", body)
		}
		w.Write([]byte(`{"workflows":[{"transitions":[{"id":"9","name":"Merge Request to Snapshot Branch","toStatusReference":"10213",
			"validators":[{"ruleKey":"system:validate-field-value","parameters":{"ruleType":"fieldRequired","fieldsRequired":"customfield_10095"}}]}]}]}`))
	}))
	defer srv.Close()
	trs, err := NewTransitionFieldsClient(newTestConfig(srv.URL)).WorkflowTransitions(context.Background(), "10039", "10143")
	if err != nil {
		t.Fatal(err)
	}
	if len(trs) != 1 || trs[0].ID != "9" || trs[0].ToStatusReference != "10213" {
		t.Fatalf("transitions = %+v", trs)
	}
	if ids, _ := trs[0].Validators[0].RequiredFields(); len(ids) != 1 || ids[0] != "customfield_10095" {
		t.Errorf("required = %v", ids)
	}
}

func TestTransitionWithFieldsAs_SendsFieldsAndMaps400(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, _, _ := r.BasicAuth(); user != userCred.Email {
			t.Errorf("auth user = %q, want the caller", user)
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errorMessages":["Fill in the Merge Request Fields Before Code Review"],"errors":{}}`))
	}))
	defer srv.Close()
	err := NewTransitionFieldsClient(newTestConfig(srv.URL)).TransitionWithFieldsAs(context.Background(), userCred, "OMS-1", "9", map[string]any{"resolution": map[string]any{"id": "1"}})
	rej := AsRejected(err)
	if rej == nil || len(rej.Messages) != 1 {
		t.Fatalf("err = %v, want a RejectedError", err)
	}
	if tr, _ := got["transition"].(map[string]any); tr["id"] != "9" {
		t.Errorf("body = %v", got)
	}
	if _, ok := got["fields"].(map[string]any)["resolution"]; !ok {
		t.Errorf("fields not sent: %v", got)
	}
}

func TestEditMetaAs_FillsFieldIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"fields":{"customfield_10095":{"required":false,"name":"Merge Request","key":"customfield_10095","schema":{"type":"string","custom":"x:textarea"}}}}`))
	}))
	defer srv.Close()
	m, err := NewTransitionFieldsClient(newTestConfig(srv.URL)).EditMetaAs(context.Background(), userCred, "OMS-1")
	if err != nil {
		t.Fatal(err)
	}
	if f := m["customfield_10095"]; f.ID() != "customfield_10095" || f.Name != "Merge Request" {
		t.Errorf("field = %+v", f)
	}
}
