package jira_client_service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"nudge/base/helpers/curl_helper"
	"nudge/config"
	"nudge/modules/jira_client/jira_client_dto"
)

// ProjectMetaClient reads a project's identity and workflow statuses and the
// site's numeric fields with the bot credential (Tech Spec "Aturan Board").
// Kept apart from JiraClientService so its many test fakes stay valid.
type ProjectMetaClient interface {
	GetProject(ctx context.Context, projectKey string) (jira_client_dto.ProjectInfo, error)
	// GetProjectStatuses: unique status names over every issue type of the
	// project (case-insensitive, first spelling wins), in Jira's order.
	GetProjectStatuses(ctx context.Context, projectKey string) ([]string, error)
	// GetNumberFields: numeric customfield_* fields, "story point" names
	// first, then by name.
	GetNumberFields(ctx context.Context) ([]jira_client_dto.NumberField, error)
	// GetCreateMetaIssueTypes / GetCreateMetaFields: the project's createmeta
	// (issue types, then every field of one type) — Aturan Board picks the
	// optional fields Buat Card shows.
	GetCreateMetaIssueTypes(ctx context.Context, projectKey string) ([]jira_client_dto.CreateMetaIssueType, error)
	GetCreateMetaFields(ctx context.Context, projectKey, typeID string) ([]jira_client_dto.CreateMetaField, error)
}

// ErrProjectNotFound is Jira's 404 for a project key.
var ErrProjectNotFound = errors.New("jira_client: project not found")

type projectMetaClient struct{ base *jiraClientServiceImpl }

func NewProjectMetaClient(cfg *config.AppConfig) ProjectMetaClient {
	return &projectMetaClient{base: &jiraClientServiceImpl{cfg: cfg}}
}

func projectErr(err error, op string) error {
	var httpErr *curl_helper.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		return ErrProjectNotFound
	}
	return fmt.Errorf("jira_client: %s failed: %w", op, err)
}

func (c *projectMetaClient) GetProject(ctx context.Context, projectKey string) (jira_client_dto.ProjectInfo, error) {
	u := fmt.Sprintf("%s/rest/api/3/project/%s", c.base.cfg.JiraBaseURL, url.PathEscape(projectKey))
	res, err := curl_helper.CurlRequest[jira_client_dto.ProjectInfo](ctx, u, c.base.authHeaders(), nil, http.MethodGet, 10000, true)
	if err != nil {
		return jira_client_dto.ProjectInfo{}, projectErr(err, "GetProject("+projectKey+")")
	}
	return *res, nil
}

func (c *projectMetaClient) GetProjectStatuses(ctx context.Context, projectKey string) ([]string, error) {
	u := fmt.Sprintf("%s/rest/api/3/project/%s/statuses", c.base.cfg.JiraBaseURL, url.PathEscape(projectKey))
	res, err := curl_helper.CurlRequest[[]jira_client_dto.ProjectIssueTypeStatuses](ctx, u, c.base.authHeaders(), nil, http.MethodGet, 10000, true)
	if err != nil {
		return nil, projectErr(err, "GetProjectStatuses("+projectKey+")")
	}
	seen := map[string]bool{}
	out := []string{}
	for _, issueType := range *res {
		for _, s := range issueType.Statuses {
			k := strings.ToLower(strings.TrimSpace(s.Name))
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, s.Name)
		}
	}
	return out, nil
}

func (c *projectMetaClient) GetCreateMetaIssueTypes(ctx context.Context, projectKey string) ([]jira_client_dto.CreateMetaIssueType, error) {
	u := fmt.Sprintf("%s/rest/api/3/issue/createmeta/%s/issuetypes?maxResults=100", c.base.cfg.JiraBaseURL, url.PathEscape(projectKey))
	page, err := curl_helper.CurlRequest[jira_client_dto.CreateMetaIssueTypesPage](ctx, u, c.base.authHeaders(), nil, http.MethodGet, 10000, true)
	if err != nil {
		return nil, projectErr(err, "GetCreateMetaIssueTypes("+projectKey+")")
	}
	return page.Items(), nil
}

func (c *projectMetaClient) GetCreateMetaFields(ctx context.Context, projectKey, typeID string) ([]jira_client_dto.CreateMetaField, error) {
	u := fmt.Sprintf("%s/rest/api/3/issue/createmeta/%s/issuetypes/%s?maxResults=200", c.base.cfg.JiraBaseURL, url.PathEscape(projectKey), url.PathEscape(typeID))
	page, err := curl_helper.CurlRequest[jira_client_dto.CreateMetaFieldsPage](ctx, u, c.base.authHeaders(), nil, http.MethodGet, 10000, true)
	if err != nil {
		return nil, projectErr(err, "GetCreateMetaFields("+projectKey+", "+typeID+")")
	}
	return page.Items(), nil
}

func (c *projectMetaClient) GetNumberFields(ctx context.Context) ([]jira_client_dto.NumberField, error) {
	res, err := curl_helper.CurlRequest[[]jira_client_dto.JiraField](ctx, c.base.cfg.JiraBaseURL+"/rest/api/3/field", c.base.authHeaders(), nil, http.MethodGet, 15000, true)
	if err != nil {
		return nil, fmt.Errorf("jira_client: GetNumberFields failed: %w", err)
	}
	out := []jira_client_dto.NumberField{}
	for _, f := range *res {
		// Only custom fields: IssueFields.Numbers keeps customfield_* only.
		if f.Schema.Type != "number" || !strings.HasPrefix(f.ID, "customfield_") {
			continue
		}
		out = append(out, jira_client_dto.NumberField{ID: f.ID, Name: f.Name, StoryPointLike: strings.Contains(strings.ToLower(f.Name), "story point")})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StoryPointLike != out[j].StoryPointLike {
			return out[i].StoryPointLike
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}
