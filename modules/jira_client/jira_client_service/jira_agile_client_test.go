package jira_client_service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchActiveSprintIssuesByAssignee_UsesOpenSprintsJQLAndSprintField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if !strings.Contains(q.Get("jql"), `assignee = "acc-1" AND sprint in openSprints()`) {
			t.Errorf("jql = %q, want open sprints for acc-1", q.Get("jql"))
		}
		for _, field := range []string{"customfield_10020", "parent", "subtasks", "issuelinks"} {
			if !strings.Contains(q.Get("fields"), field) {
				t.Errorf("fields = %q, want %s", q.Get("fields"), field)
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"issues": []map[string]any{{"key": "OMS-1", "fields": map[string]any{}}}})
	}))
	defer server.Close()

	issues, err := NewJiraAgileClient(newTestConfig(server.URL)).SearchActiveSprintIssuesByAssignee(context.Background(), nil, "acc-1")
	if err != nil || len(issues) != 1 {
		t.Fatalf("issues = %v, err = %v", issues, err)
	}
}

func TestSearchIssuesByKeysForAssignee_FiltersKeysAndAssignee(t *testing.T) {
	var jql string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jql = r.URL.Query().Get("jql")
		json.NewEncoder(w).Encode(map[string]any{"issues": []map[string]any{{"key": "OMS-11905", "fields": map[string]any{}}}})
	}))
	defer server.Close()
	client := NewJiraAgileClient(newTestConfig(server.URL))

	issues, err := client.SearchIssuesByKeysForAssignee(context.Background(), nil, []string{"OMS-11905", "HR-1", `OMS-1") OR ("x`, "CORE-77"}, "acc-1")
	if err != nil || len(issues) != 1 {
		t.Fatalf("issues = %v, err = %v", issues, err)
	}
	want := `key in (OMS-11905, CORE-77) AND assignee = "acc-1" ORDER BY updated DESC`
	if jql != want {
		t.Errorf("jql = %q\nwant  %q", jql, want)
	}

	jql = ""
	if issues, err := client.SearchIssuesByKeysForAssignee(context.Background(), nil, []string{"HR-1"}, "acc-1"); err != nil || issues != nil || jql != "" {
		t.Errorf("no valid key must skip the request: issues=%v err=%v jql=%q", issues, err, jql)
	}
}

func TestGetBoardAndActiveSprints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/agile/1.0/board/28":
			w.Write([]byte(`{"id":28,"name":"ESB OMS Board","type":"simple","location":{"projectKey":"OMS","avatarURI":"https://x/avatar/10417"}}`))
		case "/rest/agile/1.0/board/28/sprint":
			if r.URL.Query().Get("state") != "active" {
				t.Errorf("state = %q, want active", r.URL.Query().Get("state"))
			}
			w.Write([]byte(`{"values":[{"id":8294,"name":"OMS Sprint 23 - 2026","state":"active","startDate":"2026-11-09T01:00:00.000Z","endDate":"2026-11-22T10:00:00.000Z"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewJiraAgileClient(newTestConfig(server.URL))

	board, err := client.GetBoard(context.Background(), 28)
	if err != nil || board.Name != "ESB OMS Board" || board.Location.ProjectKey != "OMS" || board.Location.AvatarURI == "" {
		t.Errorf("board = %+v, err = %v", board, err)
	}
	sprints, err := client.GetActiveSprints(context.Background(), 28)
	if err != nil || len(sprints) != 1 || sprints[0].ID != 8294 {
		t.Errorf("sprints = %+v, err = %v", sprints, err)
	}
}

func TestFetchAvatar_OnlyAllowsTheJiraSiteAndImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("avatar must be fetched with the bot credentials")
		}
		if strings.HasSuffix(r.URL.Path, "/html") {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>"))
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write([]byte("<svg/>"))
	}))
	defer server.Close()
	client := NewJiraAgileClient(newTestConfig(server.URL))

	body, ct, err := client.FetchAvatar(context.Background(), server.URL+"/rest/api/2/universal_avatar/view/type/project/avatar/1")
	if err != nil || ct != "image/svg+xml" || string(body) != "<svg/>" {
		t.Errorf("FetchAvatar = %q, %q, %v", body, ct, err)
	}
	if _, _, err := client.FetchAvatar(context.Background(), "https://evil.example.com/a.png"); err == nil {
		t.Error("a URL outside the Jira site must be refused")
	}
	if _, _, err := client.FetchAvatar(context.Background(), server.URL+"/html"); err == nil {
		t.Error("a non-image response must be refused")
	}
}
