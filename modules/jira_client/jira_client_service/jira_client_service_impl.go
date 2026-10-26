package jira_client_service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"jira-u-bot/base/helpers/curl_helper"
	"jira-u-bot/config"
	"jira-u-bot/modules/jira_client/jira_client_dto"
)

type jiraClientServiceImpl struct {
	cfg *config.AppConfig
}

func NewJiraClientService(cfg *config.AppConfig) JiraClientService {
	return &jiraClientServiceImpl{cfg: cfg}
}

func (s *jiraClientServiceImpl) authHeaders() map[string]string {
	return map[string]string{
		"Authorization": basicAuthHeader(s.cfg.JiraEmail, s.cfg.JiraAPIToken),
		"Accept":        "application/json",
	}
}

func (s *jiraClientServiceImpl) SearchCodeProcessIssues(ctx context.Context) ([]jira_client_dto.Issue, error) {
	jql := fmt.Sprintf(`project in (CORE, OMS) AND status = "%s"`, jira_client_dto.StatusCodeProcess)
	params := url.Values{}
	params.Set("jql", jql)
	params.Set("fields", "summary,status,assignee,issuetype,"+jira_client_dto.StoryPointFieldID)
	params.Set("maxResults", "100")

	reqURL := fmt.Sprintf("%s/rest/api/3/search/jql?%s", s.cfg.JiraBaseURL, params.Encode())

	result, err := curl_helper.CurlRequest[jira_client_dto.SearchResponse](
		ctx, reqURL, s.authHeaders(), nil, http.MethodGet, 10000, true,
	)
	if err != nil {
		return nil, fmt.Errorf("jira_client: SearchCodeProcessIssues failed: %w", err)
	}
	return result.Issues, nil
}

func (s *jiraClientServiceImpl) GetLatestCodeProcessEntryTime(ctx context.Context, issueKey string) (time.Time, bool, error) {
	reqURL := fmt.Sprintf("%s/rest/api/3/issue/%s?expand=changelog", s.cfg.JiraBaseURL, issueKey)

	result, err := curl_helper.CurlRequest[jira_client_dto.ChangelogResponse](
		ctx, reqURL, s.authHeaders(), nil, http.MethodGet, 10000, true,
	)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("jira_client: GetLatestCodeProcessEntryTime(%s) failed: %w", issueKey, err)
	}

	var latest time.Time
	found := false
	for _, history := range result.Changelog.Histories {
		for _, item := range history.Items {
			if item.Field != "status" || item.ToString != jira_client_dto.StatusCodeProcess {
				continue
			}
			t, err := time.Parse("2006-01-02T15:04:05.000-0700", history.Created)
			if err != nil {
				continue
			}
			if !found || t.After(latest) {
				latest = t
				found = true
			}
		}
	}

	return latest, found, nil
}

func (s *jiraClientServiceImpl) LookupUserByEmail(ctx context.Context, email string) (bool, error) {
	params := url.Values{}
	params.Set("query", email)
	reqURL := fmt.Sprintf("%s/rest/api/3/user/search?%s", s.cfg.JiraBaseURL, params.Encode())

	result, err := curl_helper.CurlRequest[[]jira_client_dto.UserSearchResult](
		ctx, reqURL, s.authHeaders(), nil, http.MethodGet, 5000, true,
	)
	if err != nil {
		return false, fmt.Errorf("jira_client: LookupUserByEmail(%s) failed: %w", email, err)
	}

	for _, u := range *result {
		if u.EmailAddress == email {
			return true, nil
		}
	}
	return false, nil
}

func (s *jiraClientServiceImpl) GetIssueStatus(ctx context.Context, issueKey string) (string, error) {
	params := url.Values{}
	params.Set("fields", "status")
	reqURL := fmt.Sprintf("%s/rest/api/3/issue/%s?%s", s.cfg.JiraBaseURL, issueKey, params.Encode())

	result, err := curl_helper.CurlRequest[jira_client_dto.Issue](
		ctx, reqURL, s.authHeaders(), nil, http.MethodGet, 10000, true,
	)
	if err != nil {
		return "", fmt.Errorf("jira_client: GetIssueStatus(%s) failed: %w", issueKey, err)
	}
	return result.Fields.Status.Name, nil
}

func basicAuthHeader(username, password string) string {
	raw := username + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}
