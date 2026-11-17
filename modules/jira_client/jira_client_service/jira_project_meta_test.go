package jira_client_service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"nudge/modules/jira_client/jira_client_dto"
)

func TestProjectMetaClient_ReadsProjectStatusesAndNumberFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
			t.Errorf("missing bot credential on %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/rest/api/3/project/POS":
			json.NewEncoder(w).Encode(map[string]any{"key": "POS", "name": "ESB POS", "avatarUrls": map[string]string{"48x48": "https://jira/a.png"}})
		case "/rest/api/3/project/POS/statuses":
			json.NewEncoder(w).Encode([]map[string]any{
				{"name": "Story", "subtask": false, "statuses": []map[string]any{{"name": "To Do"}, {"name": "In Progress"}, {"name": "Done"}}},
				{"name": "Sub-task", "subtask": true, "statuses": []map[string]any{{"name": "To Do"}, {"name": "IN PROGRESS"}, {"name": "Review"}}},
			})
		case "/rest/api/3/field":
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": "summary", "name": "Summary", "schema": map[string]any{"type": "string"}},
				{"id": "timespent", "name": "Time Spent", "schema": map[string]any{"type": "number"}},
				{"id": "customfield_200", "name": "Effort", "custom": true, "schema": map[string]any{"type": "number"}},
				{"id": "customfield_10016", "name": "Story point estimate", "custom": true, "schema": map[string]any{"type": "number"}},
				{"id": "customfield_300", "name": "Team", "custom": true, "schema": map[string]any{"type": "option"}},
				{"id": "customfield_400", "name": "Notes"},
			})
		case "/rest/api/3/project/NOPE", "/rest/api/3/project/NOPE/statuses":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := NewProjectMetaClient(newTestConfig(server.URL))
	ctx := context.Background()

	p, err := c.GetProject(ctx, "POS")
	if err != nil || p.Name != "ESB POS" || p.AvatarURL() != "https://jira/a.png" {
		t.Errorf("project = %+v, %v", p, err)
	}
	statuses, err := c.GetProjectStatuses(ctx, "POS")
	if err != nil || !reflect.DeepEqual(statuses, []string{"To Do", "In Progress", "Done", "Review"}) {
		t.Errorf("statuses = %v, %v (unique over every issue type, first spelling wins)", statuses, err)
	}
	fields, err := c.GetNumberFields(ctx)
	want := []jira_client_dto.NumberField{
		{ID: "customfield_10016", Name: "Story point estimate", StoryPointLike: true},
		{ID: "customfield_200", Name: "Effort"},
	}
	if err != nil || !reflect.DeepEqual(fields, want) {
		t.Errorf("fields = %+v, %v; want numeric custom fields, story point first", fields, err)
	}
	if _, err := c.GetProject(ctx, "NOPE"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("GetProject(NOPE) err = %v", err)
	}
	if _, err := c.GetProjectStatuses(ctx, "NOPE"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("GetProjectStatuses(NOPE) err = %v", err)
	}
}

func TestProjectMetaClient_ServerErrorIsNotNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	_, err := NewProjectMetaClient(newTestConfig(server.URL)).GetProject(context.Background(), "POS")
	if err == nil || errors.Is(err, ErrProjectNotFound) {
		t.Errorf("err = %v, want a non-404 error", err)
	}
}
