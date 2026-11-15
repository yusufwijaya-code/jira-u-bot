package jira_client_service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nudge/modules/jira_client/jira_client_dto"
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

func TestChangelogEntry_DecodesAuthor(t *testing.T) {
	var resp jira_client_dto.ChangelogResponse
	raw := `{"changelog":{"histories":[{"created":"2026-11-15T10:15:00.000+0700","author":{"displayName":"Anwar"},"items":[{"field":"status","fromString":"To Do","toString":"Code Process"}]}]}}`
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if h := resp.Changelog.Histories[0]; h.Author == nil || h.Author.DisplayName != "Anwar" {
		t.Fatalf("history = %+v", h)
	}
}

// Tech Spec "Notifikasi Perubahan Status Tiket".
func TestSearchSprintIssuesUpdatedWithin_PostsRelativeJQLAndReadsEveryPage(t *testing.T) {
	var bodies []searchJQLRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var b searchJQLRequest
		json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b)
		if b.NextPageToken == "" {
			json.NewEncoder(w).Encode(map[string]any{"issues": []map[string]any{{"key": "OMS-1", "fields": map[string]any{}}}, "nextPageToken": "p2"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"issues": []map[string]any{{"key": "CORE-2", "fields": map[string]any{}}}})
	}))
	defer server.Close()

	client := NewJiraAgileClient(newTestConfig(server.URL))
	issues, err := client.SearchSprintIssuesUpdatedWithin(context.Background(), []string{"acc-1", "acc-2"}, 90*time.Second)
	if err != nil || len(issues) != 2 || len(bodies) != 2 || bodies[1].NextPageToken != "p2" {
		t.Fatalf("issues = %v, err = %v, bodies = %+v", issues, err, bodies)
	}
	for _, want := range []string{`sprint in openSprints()`, `assignee in ("acc-1","acc-2")`, `updated >= "-2m"`} {
		if !strings.Contains(bodies[0].JQL, want) {
			t.Errorf("jql = %q, want %s", bodies[0].JQL, want)
		}
	}

	bodies = nil
	if _, err := client.SearchSprintIssuesUpdatedWithin(context.Background(), []string{"acc-1"}, 0); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(bodies[0].JQL, "updated >=") {
		t.Errorf("jql = %q, want no updated filter for a full read", bodies[0].JQL)
	}
}

func TestGetRecentChangelog_ReadsTheLastPage(t *testing.T) {
	var starts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("startAt")
		starts = append(starts, start)
		created := "2026-11-01T10:00:00.000+0700"
		if start != "0" {
			created = "2026-11-15T10:00:00.000+0700"
		}
		json.NewEncoder(w).Encode(map[string]any{"total": 150,
			"values": []map[string]any{{"created": created, "items": []map[string]any{{"field": "status"}}}}})
	}))
	defer server.Close()

	got, err := NewJiraAgileClient(newTestConfig(server.URL)).GetRecentChangelog(context.Background(), "OMS-1")
	if err != nil || len(got) != 1 || got[0].Created != "2026-11-15T10:00:00.000+0700" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if len(starts) != 2 || starts[1] != "50" {
		t.Errorf("startAt = %v, want 0 then 50", starts)
	}
}
