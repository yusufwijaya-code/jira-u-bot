package issue_session_repository

import (
	"context"
	"time"

	"jira-u-bot/modules/issue_session/issue_session_dto"
)

type IssueSessionRepository interface {
	OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, storyPoints float64) error
	OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, storyPoints float64, startedAt time.Time) error
	UpdateStoryPoints(ctx context.Context, issueKey string, storyPoints float64) error
	CloseSession(ctx context.Context, issueKey string, additionalSeconds int64) error
	GetActiveSessions(ctx context.Context) ([]issue_session_dto.IssueSession, error)
	GetSessionByIssueKey(ctx context.Context, issueKey string) (*issue_session_dto.IssueSession, bool, error)
	UpdateLastOverageReminder(ctx context.Context, issueKey string) error
}
