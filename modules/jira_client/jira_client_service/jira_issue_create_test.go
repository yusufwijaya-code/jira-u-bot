package jira_client_service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nudge/modules/jira_client/jira_client_dto"
)

var userCred = &jira_client_dto.Credential{Email: "dev@esb.co.id", APIToken: "personal"}

func TestIssueTypesAs_ReadsEitherListKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/createmeta/OMS/issuetypes" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") || strings.Contains(r.Header.Get("Authorization"), "dGVzdC10b2tlbg") {
			t.Errorf("must use the personal token, got %q", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(map[string]any{"values": []map[string]any{{"id": "10143", "name": "SUB Story", "subtask": true}}})
	}))
	defer srv.Close()
	types, err := NewIssueCreateClient(newTestConfig(srv.URL)).IssueTypesAs(context.Background(), userCred, "OMS")
	if err != nil || len(types) != 1 || !types[0].Subtask || types[0].Name != "SUB Story" {
		t.Fatalf("types = %+v, err = %v", types, err)
	}
}

func TestIssueTypeFieldsAs_AsksForEveryField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("maxResults") != "200" {
			t.Errorf("maxResults = %q, want 200", r.URL.Query().Get("maxResults"))
		}
		json.NewEncoder(w).Encode(map[string]any{"fields": []map[string]any{
			{"fieldId": "customfield_1", "name": "Service", "required": true, "schema": map[string]any{"type": "option"}, "allowedValues": []map[string]any{{"id": "1", "value": "Order"}}},
		}})
	}))
	defer srv.Close()
	fields, err := NewIssueCreateClient(newTestConfig(srv.URL)).IssueTypeFieldsAs(context.Background(), userCred, "OMS", "10206")
	if err != nil || len(fields) != 1 || fields[0].ID() != "customfield_1" || fields[0].AllowedValues[0].Label() != "Order" {
		t.Fatalf("fields = %+v, err = %v", fields, err)
	}
}

func TestCreateIssueAs_ReturnsKeyAndRejectedFields(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body jira_client_dto.CreateIssueRequest
		json.NewDecoder(r.Body).Decode(&body)
		if calls == 1 {
			if body.Fields["summary"] != "Hello" {
				t.Errorf("summary = %v", body.Fields["summary"])
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": "1", "key": "CORE-99"})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"errorMessages": []string{}, "errors": map[string]string{"customfield_12001": "Merge Request is required."}})
	}))
	defer srv.Close()
	c := NewIssueCreateClient(newTestConfig(srv.URL))
	key, err := c.CreateIssueAs(context.Background(), userCred, map[string]any{"summary": "Hello"})
	if err != nil || key != "CORE-99" {
		t.Fatalf("key = %q, err = %v", key, err)
	}
	_, err = c.CreateIssueAs(context.Background(), userCred, map[string]any{"summary": "x"})
	var rej *RejectedError
	if !errors.As(err, &rej) || rej.Fields["customfield_12001"] != "Merge Request is required." {
		t.Fatalf("err = %v, want RejectedError with the field message", err)
	}
}

func TestIssueBriefAs_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := NewIssueCreateClient(newTestConfig(srv.URL)).IssueBriefAs(context.Background(), userCred, "CORE-1")
	if !errors.Is(err, ErrIssueNotFound) {
		t.Fatalf("err = %v, want ErrIssueNotFound", err)
	}
}

func TestAssignableUsersAndMyself(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/3/user/assignable/search":
			if r.URL.Query().Get("project") != "CORE" || r.URL.Query().Get("query") != "yus" {
				t.Errorf("query = %v", r.URL.Query())
			}
			json.NewEncoder(w).Encode([]map[string]any{{"accountId": "a1", "displayName": "Yusuf", "active": true}})
		case "/rest/api/3/myself":
			json.NewEncoder(w).Encode(map[string]any{"accountId": "me"})
		}
	}))
	defer srv.Close()
	c := NewIssueCreateClient(newTestConfig(srv.URL))
	users, err := c.AssignableUsersAs(context.Background(), userCred, "CORE", "yus")
	if err != nil || len(users) != 1 || users[0].AccountID != "a1" {
		t.Fatalf("users = %+v, err = %v", users, err)
	}
	me, err := c.MyselfAs(context.Background(), userCred)
	if err != nil || me != "me" {
		t.Fatalf("me = %q, err = %v", me, err)
	}
}

func TestSearchBriefsAs_DecodesTypeAndStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" || r.URL.Query().Get("jql") != `project = "CORE"` || r.URL.Query().Get("maxResults") != "50" {
			t.Errorf("request = %s ? %v", r.URL.Path, r.URL.Query())
		}
		if !strings.Contains(r.URL.Query().Get("fields"), "status") {
			t.Errorf("fields = %q, want status", r.URL.Query().Get("fields"))
		}
		json.NewEncoder(w).Encode(map[string]any{"issues": []map[string]any{{"key": "CORE-1", "fields": map[string]any{
			"summary":   "S",
			"issuetype": map[string]any{"name": "Epic", "hierarchyLevel": 1, "iconUrl": "https://i/e.svg"},
			"status":    map[string]any{"name": "In Progress", "statusCategory": map[string]any{"key": "indeterminate"}},
			"subtasks":  []map[string]any{{"key": "CORE-2"}},
		}}}})
	}))
	defer srv.Close()
	got, err := NewIssueCreateClient(newTestConfig(srv.URL)).SearchBriefsAs(context.Background(), userCred, `project = "CORE"`, 50)
	if err != nil || len(got) != 1 {
		t.Fatalf("got = %+v, err = %v", got, err)
	}
	f := got[0].Fields
	if f.IssueType.HierarchyLevel != 1 || f.IssueType.Name != "Epic" || f.IssueType.IconURL == "" || f.Status.StatusCategory.Key != "indeterminate" || len(f.Subtasks) != 1 {
		t.Fatalf("fields = %+v", f)
	}
}
