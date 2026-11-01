package issue_session_repository

import (
	"context"
	"time"

	"jira-u-bot/modules/issue_session/issue_session_dto"
)

type IssueSessionRepository interface {
	OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, storyPoints float64, assigneeEmail, issueSummary, sprintName string) error
	OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, storyPoints float64, startedAt time.Time, assigneeEmail, issueSummary, sprintName string) error
	UpdateStoryPoints(ctx context.Context, issueKey string, storyPoints float64, assigneeEmail, issueSummary, sprintName string) error
	CloseSession(ctx context.Context, issueKey string, additionalSeconds int64) error
	GetActiveSessions(ctx context.Context) ([]issue_session_dto.IssueSession, error)
	GetSessionByIssueKey(ctx context.Context, issueKey string) (*issue_session_dto.IssueSession, bool, error)
	UpdateLastOverageReminder(ctx context.Context, issueKey string) error
	// FindActiveByAssigneeEmail returns every flagActive=1 session assigned
	// to email — used by the /u-mytickets interactive command.
	FindActiveByAssigneeEmail(ctx context.Context, email string) ([]issue_session_dto.IssueSession, error)
	// FindBySprintNames is Sprint Report's data source (Tech Spec
	// "Dashboard Jira Manage", Chapter 11b, AC7.1) — includes closed
	// (flagActive=0) sessions too, unlike every other query above.
	FindBySprintNames(ctx context.Context, sprintNames []string) ([]issue_session_dto.IssueSession, error)
	// ListDistinctSprintNames populates the sprint picker (AC7.1).
	ListDistinctSprintNames(ctx context.Context) ([]string, error)
}
