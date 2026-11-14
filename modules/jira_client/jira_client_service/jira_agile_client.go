package jira_client_service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nudge/base/helpers/curl_helper"
	"nudge/config"
	"nudge/modules/jira_client/jira_client_dto"
)

// JiraAgileClient covers the Jira Agile (board/sprint) API plus the active-
// sprint search the per-board dashboard needs (Tech Spec "Per-Board Settings
// & Active Sprint Dashboard"). Kept separate from JiraClientService so the
// existing interface — and its many test fakes — stay unchanged.
type JiraAgileClient interface {
	// SearchActiveSprintIssuesByAssignee returns every CORE/OMS ticket
	// assigned to accountID that is in an open sprint, including the Sprint
	// field (for the board mapping) and budget fields.
	SearchActiveSprintIssuesByAssignee(ctx context.Context, cred *jira_client_dto.Credential, accountID string) ([]jira_client_dto.Issue, error)
	// SearchIssuesByKeysForAssignee returns those of keys that are assigned
	// to accountID, in any status — the subtasks/linked items shown as
	// dashboard branches. Keys that are malformed or outside CORE/OMS are
	// dropped; at most MaxRelatedKeys are queried.
	SearchIssuesByKeysForAssignee(ctx context.Context, cred *jira_client_dto.Credential, keys []string, accountID string) ([]jira_client_dto.Issue, error)
	// GetChangelog returns an issue's changelog histories (Tech Spec
	// "Active Sprint Tracking & Changelog Backfill").
	GetChangelog(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) ([]jira_client_dto.ChangelogEntry, error)
	// GetBoard returns a board's name, project and project avatar URL.
	GetBoard(ctx context.Context, boardID int64) (jira_client_dto.BoardInfo, error)
	// GetActiveSprints returns a board's active sprint(s) with dates.
	GetActiveSprints(ctx context.Context, boardID int64) ([]jira_client_dto.SprintRef, error)
	// FetchAvatar downloads an avatar image with the bot credentials. Only
	// URLs on the configured Jira site are allowed.
	FetchAvatar(ctx context.Context, avatarURL string) (body []byte, contentType string, err error)
}

func NewJiraAgileClient(cfg *config.AppConfig) JiraAgileClient {
	return &jiraClientServiceImpl{cfg: cfg}
}

const maxAvatarBytes = 512 * 1024

func (s *jiraClientServiceImpl) SearchActiveSprintIssuesByAssignee(ctx context.Context, cred *jira_client_dto.Credential, accountID string) ([]jira_client_dto.Issue, error) {
	jql := fmt.Sprintf(`project in (CORE, OMS) AND assignee = "%s" AND sprint in openSprints() ORDER BY updated DESC`, accountID)
	params := url.Values{}
	params.Set("jql", jql)
	params.Set("fields", dashboardIssueFields+",parent,subtasks,issuelinks")
	params.Set("maxResults", "100")

	reqURL := fmt.Sprintf("%s/rest/api/3/search/jql?%s", s.cfg.JiraBaseURL, params.Encode())
	result, err := curl_helper.CurlRequest[jira_client_dto.SearchResponse](
		ctx, reqURL, s.authHeadersFor(cred), nil, http.MethodGet, 10000, true,
	)
	if err != nil {
		return nil, fmt.Errorf("jira_client: SearchActiveSprintIssuesByAssignee(%s) failed: %w", accountID, err)
	}
	return result.Issues, nil
}

// MaxRelatedKeys caps the key list of one SearchIssuesByKeysForAssignee call.
const MaxRelatedKeys = 100

var dashboardIssueFields = "summary,status,assignee,issuetype," + jira_client_dto.StoryPointFieldID + "," +
	jira_client_dto.StoryPointFieldIDCore + "," + jira_client_dto.StoryPointFieldIDCoreEstimate + "," + jira_client_dto.SprintFieldID

func (s *jiraClientServiceImpl) SearchIssuesByKeysForAssignee(ctx context.Context, cred *jira_client_dto.Credential, keys []string, accountID string) ([]jira_client_dto.Issue, error) {
	valid := make([]string, 0, len(keys))
	for _, key := range keys {
		if jira_client_dto.IsTrackedProjectKey(key) && len(valid) < MaxRelatedKeys {
			valid = append(valid, key)
		}
	}
	if len(valid) == 0 {
		return nil, nil
	}

	jql := fmt.Sprintf(`key in (%s) AND assignee = "%s" ORDER BY updated DESC`, strings.Join(valid, ", "), accountID)
	params := url.Values{}
	params.Set("jql", jql)
	params.Set("fields", dashboardIssueFields)
	params.Set("maxResults", "100")

	reqURL := fmt.Sprintf("%s/rest/api/3/search/jql?%s", s.cfg.JiraBaseURL, params.Encode())
	result, err := curl_helper.CurlRequest[jira_client_dto.SearchResponse](
		ctx, reqURL, s.authHeadersFor(cred), nil, http.MethodGet, 10000, true,
	)
	if err != nil {
		return nil, fmt.Errorf("jira_client: SearchIssuesByKeysForAssignee(%s) failed: %w", accountID, err)
	}
	return result.Issues, nil
}

func (s *jiraClientServiceImpl) GetChangelog(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) ([]jira_client_dto.ChangelogEntry, error) {
	if !jira_client_dto.IsTrackedProjectKey(issueKey) {
		return nil, fmt.Errorf("jira_client: GetChangelog: invalid issue key %q", issueKey)
	}
	reqURL := fmt.Sprintf("%s/rest/api/3/issue/%s?expand=changelog&fields=status", s.cfg.JiraBaseURL, issueKey)
	result, err := curl_helper.CurlRequest[jira_client_dto.ChangelogResponse](
		ctx, reqURL, s.authHeadersFor(cred), nil, http.MethodGet, 10000, true,
	)
	if err != nil {
		return nil, fmt.Errorf("jira_client: GetChangelog(%s) failed: %w", issueKey, err)
	}
	return result.Changelog.Histories, nil
}

func (s *jiraClientServiceImpl) GetBoard(ctx context.Context, boardID int64) (jira_client_dto.BoardInfo, error) {
	reqURL := fmt.Sprintf("%s/rest/agile/1.0/board/%d", s.cfg.JiraBaseURL, boardID)
	result, err := curl_helper.CurlRequest[jira_client_dto.BoardInfo](
		ctx, reqURL, s.authHeaders(), nil, http.MethodGet, 10000, true,
	)
	if err != nil {
		return jira_client_dto.BoardInfo{}, fmt.Errorf("jira_client: GetBoard(%d) failed: %w", boardID, err)
	}
	return *result, nil
}

func (s *jiraClientServiceImpl) GetActiveSprints(ctx context.Context, boardID int64) ([]jira_client_dto.SprintRef, error) {
	reqURL := fmt.Sprintf("%s/rest/agile/1.0/board/%d/sprint?state=active", s.cfg.JiraBaseURL, boardID)
	result, err := curl_helper.CurlRequest[jira_client_dto.SprintPage](
		ctx, reqURL, s.authHeaders(), nil, http.MethodGet, 10000, true,
	)
	if err != nil {
		return nil, fmt.Errorf("jira_client: GetActiveSprints(%d) failed: %w", boardID, err)
	}
	return result.Values, nil
}

func (s *jiraClientServiceImpl) FetchAvatar(ctx context.Context, avatarURL string) ([]byte, string, error) {
	// Never proxy arbitrary URLs: only the configured Jira site.
	base := strings.TrimRight(s.cfg.JiraBaseURL, "/")
	if base == "" || !strings.HasPrefix(avatarURL, base+"/") {
		return nil, "", fmt.Errorf("jira_client: avatar URL is not on the Jira site")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, avatarURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", basicAuthHeader(s.cfg.JiraEmail, s.cfg.JiraAPIToken))
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("jira_client: FetchAvatar failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("jira_client: FetchAvatar returned %d", resp.StatusCode)
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("jira_client: avatar is not an image (%s)", contentType)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAvatarBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxAvatarBytes {
		return nil, "", fmt.Errorf("jira_client: avatar too large")
	}
	return body, contentType, nil
}
