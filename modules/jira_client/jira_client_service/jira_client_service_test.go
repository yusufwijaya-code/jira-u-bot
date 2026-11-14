package jira_client_service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nudge/config"
	"nudge/modules/jira_client/jira_client_dto"
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
		if jql != `(project = CORE AND status in ("Code Process", "CODE REVIEW")) OR (project = OMS AND status in ("Code Process"))` {
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

func changelogServer(t *testing.T, histories []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"changelog": map[string]any{"histories": histories}})
	}))
}

func statusChange(created, from, to string) map[string]any {
	return map[string]any{"created": created, "items": []map[string]any{{"field": "status", "fromString": from, "toString": to}}}
}

var wib = time.FixedZone("", 7*3600)

func TestGetTrackedRunTimes_OmsLatestCodeProcessEntry(t *testing.T) {
	server := changelogServer(t, []map[string]any{
		statusChange("2026-10-12T10:58:19.429+0700", "To Do", "Code Process"),
		statusChange("2026-10-20T09:00:00.000+0700", "Feedback", "Code Process"),
		statusChange("2026-10-15T09:00:00.000+0700", "Code Process", "Feedback"),
	})
	defer server.Close()

	runStart, lastEntry, found, err := NewJiraClientService(newTestConfig(server.URL)).GetTrackedRunTimes(context.Background(), "OMS-1")
	if err != nil || !found {
		t.Fatalf("found %v, err %v", found, err)
	}
	want := time.Date(2026, 10, 20, 9, 0, 0, 0, wib)
	if !runStart.Equal(want) || !lastEntry.Equal(want) {
		t.Errorf("runStart %v lastEntry %v, want both %v (the LATEST entry, not the first)", runStart, lastEntry, want)
	}
}

func TestGetTrackedRunTimes_CoreReviewDoesNotRestartTheRun(t *testing.T) {
	server := changelogServer(t, []map[string]any{
		statusChange("2026-11-10T09:00:00.000+0700", "To Do", "Code Process"),
		statusChange("2026-11-11T14:00:00.000+0700", "Code Process", "CODE REVIEW"),
	})
	defer server.Close()

	runStart, lastEntry, found, err := NewJiraClientService(newTestConfig(server.URL)).GetTrackedRunTimes(context.Background(), "CORE-1")
	if err != nil || !found {
		t.Fatalf("found %v, err %v", found, err)
	}
	if !runStart.Equal(time.Date(2026, 11, 10, 9, 0, 0, 0, wib)) {
		t.Errorf("runStart = %v, want the Code Process entry (CP → CODE REVIEW keeps the run going)", runStart)
	}
	if !lastEntry.Equal(time.Date(2026, 11, 11, 14, 0, 0, 0, wib)) {
		t.Errorf("lastEntry = %v, want the CODE REVIEW entry", lastEntry)
	}
}

func TestGetTrackedRunTimes_OmsReviewIsNotTracked(t *testing.T) {
	server := changelogServer(t, []map[string]any{
		statusChange("2026-10-12T10:58:19.429+0700", "To Do", "Code Review"),
	})
	defer server.Close()

	_, _, found, err := NewJiraClientService(newTestConfig(server.URL)).GetTrackedRunTimes(context.Background(), "OMS-1")
	if err != nil || found {
		t.Errorf("found = %v, err = %v; OMS Code Review is not tracked", found, err)
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

func TestGetTransitions_ParsesAvailableTransitions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/rest/api/3/issue/OMS-12561/transitions" {
			t.Errorf("path = %s, want /rest/api/3/issue/OMS-12561/transitions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"transitions": []map[string]any{
				{"id": "5", "name": "Carried Out Again Tomorrow or Postponed.", "to": map[string]any{"name": "To Do"}},
				{"id": "9", "name": "Merge Request to Snapshot Branch", "to": map[string]any{"name": "Code Review"}},
			},
		})
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))

	transitions, err := svc.GetTransitions(context.Background(), "OMS-12561")
	if err != nil {
		t.Fatalf("GetTransitions returned error: %v", err)
	}
	if len(transitions) != 2 {
		t.Fatalf("len(transitions) = %d, want 2", len(transitions))
	}
	if transitions[0].ID != "5" || transitions[0].To.Name != "To Do" {
		t.Errorf("transitions[0] = %+v, want {ID:5 To.Name:To Do}", transitions[0])
	}
	if transitions[1].ID != "9" || transitions[1].To.Name != "Code Review" {
		t.Errorf("transitions[1] = %+v, want {ID:9 To.Name:Code Review}", transitions[1])
	}
}

func TestGetTransitions_PropagatesErrorOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))

	_, err := svc.GetTransitions(context.Background(), "OMS-NOPE")
	if err == nil {
		t.Fatal("expected error to propagate on non-2xx response")
	}
}

func TestTransitionIssue_PostsTransitionID(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))

	if err := svc.TransitionIssue(context.Background(), "OMS-12561", "5"); err != nil {
		t.Fatalf("TransitionIssue returned error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/rest/api/3/issue/OMS-12561/transitions" {
		t.Errorf("path = %s, want /rest/api/3/issue/OMS-12561/transitions", gotPath)
	}
	transition, _ := gotBody["transition"].(map[string]any)
	if transition["id"] != "5" {
		t.Errorf("body transition.id = %v, want \"5\"", transition["id"])
	}
}

func TestTransitionIssue_PropagatesErrorOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))

	err := svc.TransitionIssue(context.Background(), "OMS-12561", "999")
	if err == nil {
		t.Fatal("expected error to propagate on non-2xx response")
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

func TestSearchAllIssuesByAssignee_NoStatusFilterInJQL(t *testing.T) {
	var gotJQL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotJQL = r.URL.Query().Get("jql")
		json.NewEncoder(w).Encode(map[string]any{"issues": []map[string]any{
			{"key": "OMS-1", "fields": map[string]any{"status": map[string]any{"name": "To Do"}, "issuetype": map[string]any{"name": "Story"}}},
			{"key": "OMS-2", "fields": map[string]any{"status": map[string]any{"name": "Done"}, "issuetype": map[string]any{"name": "Story"}}},
		}})
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))
	issues, err := svc.SearchAllIssuesByAssignee(context.Background(), nil, "acc-123")
	if err != nil {
		t.Fatalf("SearchAllIssuesByAssignee returned error: %v", err)
	}
	if strings.Contains(gotJQL, "status") {
		t.Errorf("jql = %q, must NOT filter by status (spec AC3.1 — My Board shows every status)", gotJQL)
	}
	if !strings.Contains(gotJQL, `assignee = "acc-123"`) {
		t.Errorf("jql = %q, want it scoped to assignee acc-123", gotJQL)
	}
	if len(issues) != 2 {
		t.Fatalf("len(issues) = %d, want 2", len(issues))
	}
}

func TestSearchAllIssuesByAssignee_UsesCentralCredentialWhenNil(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"issues": []map[string]any{}})
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)
	svc := NewJiraClientService(cfg)
	if _, err := svc.SearchAllIssuesByAssignee(context.Background(), nil, "acc-123"); err != nil {
		t.Fatalf("SearchAllIssuesByAssignee returned error: %v", err)
	}

	wantAuth := "Basic " + basicAuthHeaderForTest(cfg.JiraEmail, cfg.JiraAPIToken)
	if gotAuth != wantAuth {
		t.Errorf("Authorization = %q, want central credential %q", gotAuth, wantAuth)
	}
}

func TestSearchAllIssuesByAssignee_UsesPersonalCredentialWhenProvided(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"issues": []map[string]any{}})
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))
	cred := &jira_client_dto.Credential{Email: "dev@esb.co.id", APIToken: "personal-token"}
	if _, err := svc.SearchAllIssuesByAssignee(context.Background(), cred, "acc-123"); err != nil {
		t.Fatalf("SearchAllIssuesByAssignee returned error: %v", err)
	}

	wantAuth := "Basic " + basicAuthHeaderForTest(cred.Email, cred.APIToken)
	if gotAuth != wantAuth {
		t.Errorf("Authorization = %q, want personal credential %q — must not fall back to central when cred is provided", gotAuth, wantAuth)
	}
}

func TestTransitionIssueAs_UsesPersonalCredentialWhenProvided(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	svc := NewJiraClientService(newTestConfig(server.URL))
	cred := &jira_client_dto.Credential{Email: "dev@esb.co.id", APIToken: "personal-token"}
	if err := svc.TransitionIssueAs(context.Background(), cred, "OMS-1", "5"); err != nil {
		t.Fatalf("TransitionIssueAs returned error: %v", err)
	}

	wantAuth := "Basic " + basicAuthHeaderForTest(cred.Email, cred.APIToken)
	if gotAuth != wantAuth {
		t.Errorf("Authorization = %q, want personal credential %q", gotAuth, wantAuth)
	}
}

func TestGetTransitionsAs_UsesCentralCredentialWhenNil(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"transitions": []map[string]any{}})
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)
	svc := NewJiraClientService(cfg)
	if _, err := svc.GetTransitionsAs(context.Background(), nil, "OMS-1"); err != nil {
		t.Fatalf("GetTransitionsAs returned error: %v", err)
	}

	wantAuth := "Basic " + basicAuthHeaderForTest(cfg.JiraEmail, cfg.JiraAPIToken)
	if gotAuth != wantAuth {
		t.Errorf("Authorization = %q, want central credential %q", gotAuth, wantAuth)
	}
}

func basicAuthHeaderForTest(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}
