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
	LookupUserByEmail(ctx context.Context, email string) (bool, error)
}
