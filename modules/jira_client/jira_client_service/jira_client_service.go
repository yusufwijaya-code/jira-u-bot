package jira_client_service

import (
	"context"
	"time"

	"jira-u-bot/modules/jira_client/jira_client_dto"
)

type JiraClientService interface {
	SearchCodeProcessIssues(ctx context.Context) ([]jira_client_dto.Issue, error)
	// GetLatestCodeProcessEntryTime returns the timestamp of the MOST RECENT
	// transition into "Code Process" for issueKey — not the first one ever.
	// Used only at poller startup to backfill tickets already mid-flight
	// (spec Business Logic point 7); steady-state polling never calls this.
	GetLatestCodeProcessEntryTime(ctx context.Context, issueKey string) (time.Time, bool, error)
	// LookupUserByEmail reports whether a Jira account matches email and,
	// if so, its accountId — the canonical identifier for matching
	// registered users against issue assignees later (Jira hides
	// emailAddress on assignee objects org-wide, but accountId is always
	// present). accountID is empty when found is false.
	LookupUserByEmail(ctx context.Context, email string) (accountID string, found bool, err error)
	// GetIssueStatus fetches issueKey's CURRENT status directly — used only
	// at poller exit-event time (spec Business Logic point 2), since by
	// then the ticket no longer appears in any "status = Code Process" JQL
	// result and its new status can't be learned any other way.
	GetIssueStatus(ctx context.Context, issueKey string) (string, error)
}
