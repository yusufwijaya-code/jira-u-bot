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

// TransitionFieldsClient reads what a move requires and writes the fields
// it needs ("move-fields"). Kept apart from JiraClientService so existing
// fakes of that interface stay valid.
type TransitionFieldsClient interface {
	// WorkflowTransitions reads the workflow of (project, issue type) with
	// its transition validators. It runs with the central (operator)
	// credential: the endpoint may need admin rights a developer lacks.
	WorkflowTransitions(ctx context.Context, projectID, issueTypeID string) ([]jira_client_dto.WorkflowTransition, error)
	// ScreenTransitionsAs is GET transitions with their screen fields.
	ScreenTransitionsAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) ([]jira_client_dto.ScreenTransition, error)
	// EditMetaAs: the fields cred may edit on the issue, keyed by id.
	EditMetaAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) (map[string]jira_client_dto.CreateMetaField, error)
	// FieldValuesAs reads the issue's project/type ids and the given fields' raw values.
	FieldValuesAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey string, fieldIDs []string) (jira_client_dto.IssueFieldValues, error)
	// EditIssueAs sets fields on the issue; a 400 is a *RejectedError.
	EditIssueAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey string, fields map[string]any) error
	// TransitionWithFieldsAs runs the transition, with screen fields when
	// given; a 400 is a *RejectedError.
	TransitionWithFieldsAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey, transitionID string, fields map[string]any) error
}

type transitionFieldsClient struct{ base *jiraClientServiceImpl }

func NewTransitionFieldsClient(cfg *config.AppConfig) TransitionFieldsClient {
	return &transitionFieldsClient{base: &jiraClientServiceImpl{cfg: cfg}}
}

func (c *transitionFieldsClient) url(path string) string { return c.base.cfg.JiraBaseURL + path }

func (c *transitionFieldsClient) WorkflowTransitions(ctx context.Context, projectID, issueTypeID string) ([]jira_client_dto.WorkflowTransition, error) {
	body := jira_client_dto.WorkflowSearchRequest{ProjectAndIssueTypes: []jira_client_dto.ProjectIssueType{{ProjectID: projectID, IssueTypeID: issueTypeID}}}
	res, err := curl_helper.CurlRequest[jira_client_dto.WorkflowsResponse](ctx, c.url("/rest/api/3/workflows?expand=values.transitions"), c.base.authHeadersFor(nil), body, http.MethodPost, 10000, true)
	if err != nil {
		return nil, fmt.Errorf("jira_client: WorkflowTransitions(%s, %s) failed: %w", projectID, issueTypeID, err)
	}
	var out []jira_client_dto.WorkflowTransition
	for _, w := range res.Workflows {
		out = append(out, w.Transitions...)
	}
	return out, nil
}

func (c *transitionFieldsClient) ScreenTransitionsAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) ([]jira_client_dto.ScreenTransition, error) {
	u := c.url(fmt.Sprintf("/rest/api/3/issue/%s/transitions?expand=transitions.fields", url.PathEscape(issueKey)))
	res, err := curl_helper.CurlRequest[jira_client_dto.ScreenTransitionsResponse](ctx, u, c.base.authHeadersFor(cred), nil, http.MethodGet, 10000, true)
	if err != nil {
		return nil, fmt.Errorf("jira_client: ScreenTransitionsAs(%s) failed: %w", issueKey, err)
	}
	for i := range res.Transitions {
		fillFieldIDs(res.Transitions[i].Fields)
	}
	return res.Transitions, nil
}

func (c *transitionFieldsClient) EditMetaAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) (map[string]jira_client_dto.CreateMetaField, error) {
	u := c.url(fmt.Sprintf("/rest/api/3/issue/%s/editmeta", url.PathEscape(issueKey)))
	res, err := curl_helper.CurlRequest[jira_client_dto.EditMetaResponse](ctx, u, c.base.authHeadersFor(cred), nil, http.MethodGet, 10000, true)
	if err != nil {
		return nil, fmt.Errorf("jira_client: EditMetaAs(%s) failed: %w", issueKey, err)
	}
	fillFieldIDs(res.Fields)
	return res.Fields, nil
}

// fillFieldIDs: editmeta / screen fields are keyed by id, the entries
// themselves may carry only "key".
func fillFieldIDs(fields map[string]jira_client_dto.CreateMetaField) {
	for id, f := range fields {
		f.FieldID = id
		fields[id] = f
	}
}

func (c *transitionFieldsClient) FieldValuesAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey string, fieldIDs []string) (jira_client_dto.IssueFieldValues, error) {
	q := url.Values{"fields": {strings.Join(append([]string{"project", "issuetype"}, fieldIDs...), ",")}}
	u := c.url(fmt.Sprintf("/rest/api/3/issue/%s?%s", url.PathEscape(issueKey), q.Encode()))
	res, err := curl_helper.CurlRequest[jira_client_dto.IssueFieldValues](ctx, u, c.base.authHeadersFor(cred), nil, http.MethodGet, 10000, true)
	if err != nil {
		return jira_client_dto.IssueFieldValues{}, fmt.Errorf("jira_client: FieldValuesAs(%s) failed: %w", issueKey, err)
	}
	return *res, nil
}

func (c *transitionFieldsClient) EditIssueAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey string, fields map[string]any) error {
	u := c.url(fmt.Sprintf("/rest/api/3/issue/%s", url.PathEscape(issueKey)))
	_, err := curl_helper.CurlRequest[struct{}](ctx, u, c.base.authHeadersFor(cred), jira_client_dto.EditIssueRequest{Fields: fields}, http.MethodPut, 15000, false)
	if err != nil {
		if rej := asRejected(err); rej != nil {
			return rej
		}
		return fmt.Errorf("jira_client: EditIssueAs(%s) failed: %w", issueKey, err)
	}
	return nil
}

func (c *transitionFieldsClient) TransitionWithFieldsAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey, transitionID string, fields map[string]any) error {
	u := c.url(fmt.Sprintf("/rest/api/3/issue/%s/transitions", url.PathEscape(issueKey)))
	body := jira_client_dto.TransitionWithFieldsRequest{Transition: jira_client_dto.TransitionID{ID: transitionID}, Fields: fields}
	_, err := curl_helper.CurlRequest[struct{}](ctx, u, c.base.authHeadersFor(cred), body, http.MethodPost, 10000, false)
	if err != nil {
		if rej := asRejected(err); rej != nil {
			return rej
		}
		return fmt.Errorf("jira_client: TransitionWithFieldsAs(%s, %s) failed: %w", issueKey, transitionID, err)
	}
	return nil
}

// AsRejected exposes asRejected for callers holding a plain transition error.
func AsRejected(err error) *RejectedError {
	var rej *RejectedError
	if errors.As(err, &rej) {
		return rej
	}
	return asRejected(err)
}

// asRejected turns a Jira 400 with an errors/errorMessages body into a
// *RejectedError (nil otherwise).
func asRejected(err error) *RejectedError {
	var httpErr *curl_helper.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusBadRequest {
		return nil
	}
	var body jira_client_dto.JiraErrorBody
	if json.Unmarshal([]byte(httpErr.Body), &body) != nil || (len(body.Errors) == 0 && len(body.ErrorMessages) == 0) {
		return nil
	}
	return &RejectedError{Fields: body.Errors, Messages: body.ErrorMessages}
}
