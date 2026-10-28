package jira_client_service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jira-u-bot/config"
)

func newTestConfig(baseURL string) *config.AppConfig {
	return &config.AppConfig{
		JiraBaseURL:  baseURL,
		JiraEmail:    "bot@esb.co.id",
		JiraAPIToken: "test-token",
	}
}

func TestSearchCodeProcessIssues_ParsesIssueList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("path = %s, want /rest/api/3/search/jql", r.URL.Path)
		}
		jql := r.URL.Query().Get("jql")
		if jql != `project in (CORE, OMS) AND status = "Code Process"` {
			t.Errorf("jql = %q, unexpected", jql)
		}

		sp := 3.0
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"issues": []map[string]any{
				{
					"key": "OMS-1",
					"fields": map[string]any{
						"status":            map[string]any{"name": "Code Process"},
						"assignee":          map[string]any{"emailAddress": "anwar@esb.co.id", "displayName": "Anwar"},
						"issuetype":         map[string]any{"name": "Story"},
						"customfield_10016": sp,
					},
				},
			},
		})
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))

	issues, err := svc.SearchCodeProcessIssues(context.Background())
	if err != nil {
		t.Fatalf("SearchCodeProcessIssues returned error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(issues))
	}
	if issues[0].Key != "OMS-1" {
		t.Errorf("Key = %q, want OMS-1", issues[0].Key)
	}
	if issues[0].Fields.Assignee == nil || issues[0].Fields.Assignee.EmailAddress != "anwar@esb.co.id" {
		t.Errorf("Assignee = %+v, want emailAddress anwar@esb.co.id", issues[0].Fields.Assignee)
	}
	if issues[0].Fields.StoryPoints == nil || *issues[0].Fields.StoryPoints != 3.0 {
		t.Errorf("StoryPoints = %v, want 3.0", issues[0].Fields.StoryPoints)
	}
}

func TestSearchCodeProcessIssues_SendsBasicAuthHeader(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"issues": []map[string]any{}})
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))
	if _, err := svc.SearchCodeProcessIssues(context.Background()); err != nil {
		t.Fatalf("SearchCodeProcessIssues returned error: %v", err)
	}

	if len(gotAuth) < 6 || gotAuth[:6] != "Basic " {
		t.Errorf("Authorization header = %q, want a Basic auth header", gotAuth)
	}
}

func TestGetLatestCodeProcessEntryTime_FindsMostRecentTransition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"changelog": map[string]any{
				"histories": []map[string]any{
					{
						"created": "2026-10-12T10:58:19.429+0700",
						"items": []map[string]any{
							{"field": "status", "fromString": "To Do", "toString": "Code Process"},
						},
					},
					{
						"created": "2026-10-20T09:00:00.000+0700",
						"items": []map[string]any{
							{"field": "status", "fromString": "Feedback", "toString": "Code Process"},
						},
					},
					{
						"created": "2026-10-15T09:00:00.000+0700",
						"items": []map[string]any{
							{"field": "status", "fromString": "Code Process", "toString": "Feedback"},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))

	entryTime, found, err := svc.GetLatestCodeProcessEntryTime(context.Background(), "OMS-1")
	if err != nil {
		t.Fatalf("GetLatestCodeProcessEntryTime returned error: %v", err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}

	want := time.Date(2026, 10, 20, 9, 0, 0, 0, time.FixedZone("", 7*3600))
	if !entryTime.Equal(want) {
		t.Errorf("entryTime = %v, want %v (the LATEST entry into Code Process, not the first — there are two entry events here, 10-12 and 10-20)", entryTime, want)
	}
}

func TestGetLatestCodeProcessEntryTime_NotFoundWhenNeverEnteredCodeProcess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"changelog": map[string]any{
				"histories": []map[string]any{
					{
						"created": "2026-10-12T10:58:19.429+0700",
						"items": []map[string]any{
							{"field": "status", "fromString": "To Do", "toString": "Code Review"},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))

	_, found, err := svc.GetLatestCodeProcessEntryTime(context.Background(), "OMS-1")
	if err != nil {
		t.Fatalf("GetLatestCodeProcessEntryTime returned error: %v", err)
	}
	if found {
		t.Error("found = true, want false — no history item transitions into Code Process")
	}
}

func TestLookupUserByEmail_TrueWhenExactEmailMatchFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"accountId": "abc123", "emailAddress": "anwar@esb.co.id"},
		})
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))

	accountID, found, err := svc.LookupUserByEmail(context.Background(), "anwar@esb.co.id")
	if err != nil {
		t.Fatalf("LookupUserByEmail returned error: %v", err)
	}
	if !found {
		t.Error("found = false, want true")
	}
	if accountID != "abc123" {
		t.Errorf("accountID = %q, want %q", accountID, "abc123")
	}
}

func TestLookupUserByEmail_FalseWhenNoResultsReturned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))

	accountID, found, err := svc.LookupUserByEmail(context.Background(), "nobody@esb.co.id")
	if err != nil {
		t.Fatalf("LookupUserByEmail returned error: %v", err)
	}
	if found {
		t.Error("found = true, want false for an email with no matching Jira account")
	}
	if accountID != "" {
		t.Errorf("accountID = %q, want empty when not found", accountID)
	}
}

func TestLookupUserByEmail_PropagatesHTTPErrorOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))

	_, _, err := svc.LookupUserByEmail(context.Background(), "anwar@esb.co.id")
	if err == nil {
		t.Fatal("expected error to propagate when Jira API returns 500 (spec: hard-block registration treats lookup failure as rejection, Note C5)")
	}
}

func TestGetIssueStatus_ReturnsCurrentStatusName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/OMS-1" {
			t.Errorf("path = %s, want /rest/api/3/issue/OMS-1", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"fields": map[string]any{"status": map[string]any{"name": "Code Review"}},
		})
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))
	status, err := svc.GetIssueStatus(context.Background(), "OMS-1")
	if err != nil {
		t.Fatalf("GetIssueStatus returned error: %v", err)
	}
	if status != "Code Review" {
		t.Errorf("status = %q, want %q", status, "Code Review")
	}
}

func TestGetIssueStatus_PropagatesErrorOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))
	_, err := svc.GetIssueStatus(context.Background(), "OMS-1")
	if err == nil {
		t.Fatal("expected error to propagate when Jira API returns 500")
	}
}

func TestLookupUserByEmail_TrueWhenResultFoundButEmailAddressHidden(t *testing.T) {
	// Jira Cloud can mask emailAddress in search results for privacy reasons
	// even when the account is a real match (confirmed live against the real
	// esb-id Jira site: status 200, one result, emailAddress == "") — a match
	// must not depend on that field being populated.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"accountId": "712020:cc15b1b8-2e0f-46fb-bea5-9fcb502354cf", "emailAddress": "", "displayName": "Benni Setiady Gunawan Simarmata"},
		})
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))

	accountID, found, err := svc.LookupUserByEmail(context.Background(), "benni.simarmata@esb.co.id")
	if err != nil {
		t.Fatalf("LookupUserByEmail returned error: %v", err)
	}
	if !found {
		t.Error("found = false, want true — a returned result is a match even when emailAddress is hidden")
	}
	if accountID != "712020:cc15b1b8-2e0f-46fb-bea5-9fcb502354cf" {
		t.Errorf("accountID = %q, want the match's accountId even when emailAddress is hidden", accountID)
	}
}

func TestSearchCodeProcessIssues_RequestsBothStoryPointFieldsAndParsesCoreVariant(t *testing.T) {
	// CORE project tickets use a different custom field than OMS for Story
	// Points (confirmed live: OMS uses customfield_10016 "Story point
	// estimate", CORE uses customfield_11053 "Story Points" — a ticket
	// never has both populated).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields := r.URL.Query().Get("fields")
		if !strings.Contains(fields, "customfield_10016") || !strings.Contains(fields, "customfield_11053") || !strings.Contains(fields, "customfield_10125") {
			t.Errorf("fields = %q, want it to request customfield_10016, customfield_11053, and customfield_10125", fields)
		}

		sp := 1.5
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"issues": []map[string]any{
				{
					"key": "CORE-10782",
					"fields": map[string]any{
						"status":            map[string]any{"name": "Code Process"},
						"assignee":          map[string]any{"emailAddress": "rio.pratama@esb.co.id", "displayName": "Rio"},
						"issuetype":         map[string]any{"name": "Subtask"},
						"customfield_11053": sp,
					},
				},
			},
		})
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))

	issues, err := svc.SearchCodeProcessIssues(context.Background())
	if err != nil {
		t.Fatalf("SearchCodeProcessIssues returned error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(issues))
	}
	if issues[0].Fields.StoryPoints != nil {
		t.Errorf("StoryPoints (customfield_10016) = %v, want nil for a CORE ticket", *issues[0].Fields.StoryPoints)
	}
	if issues[0].Fields.StoryPointsCore == nil || *issues[0].Fields.StoryPointsCore != 1.5 {
		t.Errorf("StoryPointsCore (customfield_11053) = %v, want 1.5", issues[0].Fields.StoryPointsCore)
	}
	if got := issues[0].Fields.EffectiveStoryPoints(); got == nil || *got != 1.5 {
		t.Errorf("EffectiveStoryPoints() = %v, want 1.5", got)
	}
}

func TestSearchCodeProcessIssues_ParsesCoreStoryPointEstimateField(t *testing.T) {
	// Reproduces CORE-10832: a CORE ticket can have "Story Point Estimate"
	// (customfield_10125) filled in while "Story Points" (customfield_11053)
	// stays empty — EffectiveStoryPoints must still resolve a value instead
	// of silently treating the ticket as having no budget.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"issues": []map[string]any{
				{
					"key": "CORE-10832",
					"fields": map[string]any{
						"status":            map[string]any{"name": "Code Process"},
						"assignee":          map[string]any{"emailAddress": "kevin.adam@esb.co.id", "displayName": "Kevin"},
						"issuetype":         map[string]any{"name": "Bug"},
						"customfield_10125": 3,
					},
				},
			},
		})
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))

	issues, err := svc.SearchCodeProcessIssues(context.Background())
	if err != nil {
		t.Fatalf("SearchCodeProcessIssues returned error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(issues))
	}
	if got := issues[0].Fields.EffectiveStoryPoints(); got == nil || *got != 3 {
		t.Errorf("EffectiveStoryPoints() = %v, want 3 — Story Point Estimate must be recognized even when Story Points is empty", got)
	}
}
