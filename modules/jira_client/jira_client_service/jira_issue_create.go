package jira_client_service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"nudge/base/helpers/curl_helper"
	"nudge/config"
	"nudge/modules/jira_client/jira_client_dto"
)

// IssueCreateClient creates Jira issues with the caller's own token (Tech
// Spec "Papan Tiket Revamp & Buat Card Jira", Phase 2). Kept apart from
// JiraClientService so existing fakes of that interface stay valid.
type IssueCreateClient interface {
	IssueTypesAs(ctx context.Context, cred *jira_client_dto.Credential, projectKey string) ([]jira_client_dto.CreateMetaIssueType, error)
	IssueTypeFieldsAs(ctx context.Context, cred *jira_client_dto.Credential, projectKey, typeID string) ([]jira_client_dto.CreateMetaField, error)
	CreateIssueAs(ctx context.Context, cred *jira_client_dto.Credential, fields map[string]any) (string, error)
	AssignableUsersAs(ctx context.Context, cred *jira_client_dto.Credential, projectKey, query string) ([]jira_client_dto.AssignableUser, error)
	IssueBriefAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) (jira_client_dto.IssueBrief, error)
	SearchBriefsAs(ctx context.Context, cred *jira_client_dto.Credential, jql string, maxResults int) ([]jira_client_dto.IssueBrief, error)
	MyselfAs(ctx context.Context, cred *jira_client_dto.Credential) (string, error)
}

var ErrIssueNotFound = errors.New("jira_client: issue not found")

// RejectedError is a 400 from Jira on create: per-field and general messages.
type RejectedError struct {
	Fields   map[string]string
	Messages []string
}

func (e *RejectedError) Error() string {
	parts := append([]string{}, e.Messages...)
	for k, v := range e.Fields {
		parts = append(parts, k+": "+v)
	}
	return "jira rejected the issue: " + strings.Join(parts, "; ")
}

type issueCreateClient struct{ base *jiraClientServiceImpl }

func NewIssueCreateClient(cfg *config.AppConfig) IssueCreateClient {
	return &issueCreateClient{base: &jiraClientServiceImpl{cfg: cfg}}
}

func (c *issueCreateClient) url(path string) string { return c.base.cfg.JiraBaseURL + path }

func (c *issueCreateClient) IssueTypesAs(ctx context.Context, cred *jira_client_dto.Credential, projectKey string) ([]jira_client_dto.CreateMetaIssueType, error) {
	u := c.url(fmt.Sprintf("/rest/api/3/issue/createmeta/%s/issuetypes?maxResults=100", url.PathEscape(projectKey)))
	page, err := curl_helper.CurlRequest[jira_client_dto.CreateMetaIssueTypesPage](ctx, u, c.base.authHeadersFor(cred), nil, http.MethodGet, 10000, true)
	if err != nil {
		return nil, fmt.Errorf("jira_client: IssueTypesAs(%s) failed: %w", projectKey, err)
	}
	return page.Items(), nil
}

func (c *issueCreateClient) IssueTypeFieldsAs(ctx context.Context, cred *jira_client_dto.Credential, projectKey, typeID string) ([]jira_client_dto.CreateMetaField, error) {
	u := c.url(fmt.Sprintf("/rest/api/3/issue/createmeta/%s/issuetypes/%s?maxResults=200", url.PathEscape(projectKey), url.PathEscape(typeID)))
	page, err := curl_helper.CurlRequest[jira_client_dto.CreateMetaFieldsPage](ctx, u, c.base.authHeadersFor(cred), nil, http.MethodGet, 10000, true)
	if err != nil {
		return nil, fmt.Errorf("jira_client: IssueTypeFieldsAs(%s, %s) failed: %w", projectKey, typeID, err)
	}
	return page.Items(), nil
}

func (c *issueCreateClient) CreateIssueAs(ctx context.Context, cred *jira_client_dto.Credential, fields map[string]any) (string, error) {
	res, err := curl_helper.CurlRequest[jira_client_dto.CreateIssueResponse](ctx, c.url("/rest/api/3/issue"), c.base.authHeadersFor(cred), jira_client_dto.CreateIssueRequest{Fields: fields}, http.MethodPost, 15000, true)
	if err != nil {
		var httpErr *curl_helper.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusBadRequest {
			var body jira_client_dto.JiraErrorBody
			if json.Unmarshal([]byte(httpErr.Body), &body) == nil && (len(body.Errors) > 0 || len(body.ErrorMessages) > 0) {
				return "", &RejectedError{Fields: body.Errors, Messages: body.ErrorMessages}
			}
		}
		return "", fmt.Errorf("jira_client: CreateIssueAs failed: %w", err)
	}
	return res.Key, nil
}

func (c *issueCreateClient) AssignableUsersAs(ctx context.Context, cred *jira_client_dto.Credential, projectKey, query string) ([]jira_client_dto.AssignableUser, error) {
	q := url.Values{"project": {projectKey}, "query": {query}, "maxResults": {"10"}}
	res, err := curl_helper.CurlRequest[[]jira_client_dto.AssignableUser](ctx, c.url("/rest/api/3/user/assignable/search?"+q.Encode()), c.base.authHeadersFor(cred), nil, http.MethodGet, 10000, true)
	if err != nil {
		return nil, fmt.Errorf("jira_client: AssignableUsersAs(%s) failed: %w", projectKey, err)
	}
	return *res, nil
}

func (c *issueCreateClient) IssueBriefAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) (jira_client_dto.IssueBrief, error) {
	u := c.url(fmt.Sprintf("/rest/api/3/issue/%s?fields=%s", url.PathEscape(issueKey), jira_client_dto.IssueBriefFields))
	res, err := curl_helper.CurlRequest[jira_client_dto.IssueBrief](ctx, u, c.base.authHeadersFor(cred), nil, http.MethodGet, 10000, true)
	if err != nil {
		var httpErr *curl_helper.HTTPError
		if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusNotFound || httpErr.StatusCode == http.StatusForbidden) {
			return jira_client_dto.IssueBrief{}, ErrIssueNotFound
		}
		return jira_client_dto.IssueBrief{}, fmt.Errorf("jira_client: IssueBriefAs(%s) failed: %w", issueKey, err)
	}
	return *res, nil
}

func (c *issueCreateClient) SearchBriefsAs(ctx context.Context, cred *jira_client_dto.Credential, jql string, maxResults int) ([]jira_client_dto.IssueBrief, error) {
	q := url.Values{"jql": {jql}, "fields": {jira_client_dto.IssueBriefFields}, "maxResults": {fmt.Sprint(maxResults)}}
	res, err := curl_helper.CurlRequest[jira_client_dto.IssueBriefPage](ctx, c.url("/rest/api/3/search/jql?"+q.Encode()), c.base.authHeadersFor(cred), nil, http.MethodGet, 10000, true)
	if err != nil {
		return nil, fmt.Errorf("jira_client: SearchBriefsAs failed: %w", err)
	}
	return res.Issues, nil
}

func (c *issueCreateClient) MyselfAs(ctx context.Context, cred *jira_client_dto.Credential) (string, error) {
	res, err := curl_helper.CurlRequest[jira_client_dto.Myself](ctx, c.url("/rest/api/3/myself"), c.base.authHeadersFor(cred), nil, http.MethodGet, 10000, true)
	if err != nil {
		return "", fmt.Errorf("jira_client: MyselfAs failed: %w", err)
	}
	return res.AccountID, nil
}
