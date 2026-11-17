package jira_client_service

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
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
	// SearchActiveSprintIssuesByAssignee returns every ticket of every
	// project with an active tracking rule
	// assigned to accountID that is in an open sprint, including the Sprint
	// field (for the board mapping) and budget fields.
	SearchActiveSprintIssuesByAssignee(ctx context.Context, cred *jira_client_dto.Credential, accountID string) ([]jira_client_dto.Issue, error)
	// SearchIssuesByKeysForAssignee returns those of keys that are assigned
	// to accountID, in any status — the subtasks/linked items shown as
	// dashboard branches. Keys that are malformed or outside every project
	// with an active tracking rule are
	// dropped; at most MaxRelatedKeys are queried.
	SearchIssuesByKeysForAssignee(ctx context.Context, cred *jira_client_dto.Credential, keys []string, accountID string) ([]jira_client_dto.Issue, error)
	// SearchSprintIssuesUpdatedWithin returns the active-sprint tickets of
	// every project with an active tracking rule of any of accountIDs (bot credential); within > 0 keeps only
	// those updated in that last stretch (relative JQL, so the bot
	// profile's timezone doesn't matter) — the status watcher's query (Tech
	// Spec "Notifikasi Perubahan Status Tiket"). POST, every page read.
	SearchSprintIssuesUpdatedWithin(ctx context.Context, accountIDs []string, within time.Duration) ([]jira_client_dto.Issue, error)
	// GetRecentChangelog returns the newest page (up to 100 entries) of an
	// issue's changelog — expand=changelog only returns the oldest ones.
	GetRecentChangelog(ctx context.Context, issueKey string) ([]jira_client_dto.ChangelogEntry, error)
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
	projects := jira_client_dto.ProjectsJQLIn()
	if projects == "" {
		return nil, nil
	}
	jql := fmt.Sprintf(`%s AND assignee = "%s" AND sprint in openSprints() ORDER BY updated DESC`, projects, accountID)
	params := url.Values{}
	params.Set("jql", jql)
	params.Set("fields", issueFieldsParam(true)+",parent,subtasks,issuelinks")
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

// issueFieldsParam is the fields= of every issue search: the basics, the
// Story Point fields of every active tracking rule and, withSprint, the
// Sprint field.
func issueFieldsParam(withSprint bool) string {
	fields := append([]string{"summary", "status", "assignee", "issuetype"}, jira_client_dto.StoryPointFieldIDs()...)
	if withSprint {
		fields = append(fields, jira_client_dto.SprintFieldID)
	}
	return strings.Join(fields, ",")
}

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
	params.Set("fields", issueFieldsParam(true))
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

// statusWatchMaxPages bounds one status-watch search (100 issues a page).
const statusWatchMaxPages = 20

type searchJQLRequest struct {
	JQL           string   `json:"jql"`
	Fields        []string `json:"fields"`
	MaxResults    int      `json:"maxResults"`
	NextPageToken string   `json:"nextPageToken,omitempty"`
}

func (s *jiraClientServiceImpl) SearchSprintIssuesUpdatedWithin(ctx context.Context, accountIDs []string, within time.Duration) ([]jira_client_dto.Issue, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	quoted := make([]string, len(accountIDs))
	for i, id := range accountIDs {
		quoted[i] = strconv.Quote(id)
	}
	projects := jira_client_dto.ProjectsJQLIn()
	if projects == "" {
		return nil, nil
	}
	jql := fmt.Sprintf(`%s AND sprint in openSprints() AND assignee in (%s)`, projects, strings.Join(quoted, ","))
	if within > 0 {
		minutes := int(math.Ceil(within.Minutes()))
		jql += fmt.Sprintf(` AND updated >= "-%dm"`, minutes)
	}
	jql += " ORDER BY updated DESC"

	// POST: the accountId list would make a GET URL too long as users grow.
	reqURL := fmt.Sprintf("%s/rest/api/3/search/jql", s.cfg.JiraBaseURL)
	body := searchJQLRequest{JQL: jql, Fields: []string{"summary", "status", "assignee", jira_client_dto.SprintFieldID}, MaxResults: 100}
	var out []jira_client_dto.Issue
	for page := 0; page < statusWatchMaxPages; page++ {
		result, err := curl_helper.CurlRequest[jira_client_dto.SearchResponse](
			ctx, reqURL, s.authHeadersFor(nil), body, http.MethodPost, 10000, true,
		)
		if err != nil {
			return nil, fmt.Errorf("jira_client: SearchSprintIssuesUpdatedWithin failed: %w", err)
		}
		out = append(out, result.Issues...)
		if result.NextPageToken == "" {
			return out, nil
		}
		body.NextPageToken = result.NextPageToken
	}
	log.Printf("jira_client: status-watch search stopped at %d issues (%d pages); older ones were not read", len(out), statusWatchMaxPages)
	return out, nil
}

type changelogPage struct {
	StartAt    int                              `json:"startAt"`
	MaxResults int                              `json:"maxResults"`
	Total      int                              `json:"total"`
	Values     []jira_client_dto.ChangelogEntry `json:"values"`
}

func (s *jiraClientServiceImpl) GetRecentChangelog(ctx context.Context, issueKey string) ([]jira_client_dto.ChangelogEntry, error) {
	if !jira_client_dto.IsTrackedProjectKey(issueKey) {
		return nil, fmt.Errorf("jira_client: GetRecentChangelog: invalid issue key %q", issueKey)
	}
	const pageSize = 100
	fetch := func(startAt int) (*changelogPage, error) {
		reqURL := fmt.Sprintf("%s/rest/api/3/issue/%s/changelog?startAt=%d&maxResults=%d", s.cfg.JiraBaseURL, issueKey, startAt, pageSize)
		return curl_helper.CurlRequest[changelogPage](ctx, reqURL, s.authHeadersFor(nil), nil, http.MethodGet, 10000, true)
	}
	page, err := fetch(0)
	if err != nil {
		return nil, fmt.Errorf("jira_client: GetRecentChangelog(%s) failed: %w", issueKey, err)
	}
	// Entries are oldest first: when there are more, read the last page.
	if page.Total > len(page.Values) {
		last, err := fetch(page.Total - pageSize)
		if err != nil {
			return nil, fmt.Errorf("jira_client: GetRecentChangelog(%s) failed: %w", issueKey, err)
		}
		return last.Values, nil
	}
	return page.Values, nil
}
