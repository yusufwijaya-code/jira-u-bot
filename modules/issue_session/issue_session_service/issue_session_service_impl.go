package issue_session_service

import (
	"context"
	"time"

	"jira-u-bot/modules/issue_session/issue_session_dto"
	"jira-u-bot/modules/issue_session/issue_session_repository"
)

type issueSessionServiceImpl struct {
	repo issue_session_repository.IssueSessionRepository
}

func NewIssueSessionService(repo issue_session_repository.IssueSessionRepository) IssueSessionService {
	return &issueSessionServiceImpl{repo: repo}
}

func (s *issueSessionServiceImpl) OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, storyPoints float64, assigneeEmail, issueSummary string) error {
	return s.repo.OpenOrResumeSession(ctx, issueKey, projectKey, storyPoints, assigneeEmail, issueSummary)
}

func (s *issueSessionServiceImpl) OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, storyPoints float64, startedAt time.Time, assigneeEmail, issueSummary string) error {
	return s.repo.OpenOrResumeSessionAt(ctx, issueKey, projectKey, storyPoints, startedAt, assigneeEmail, issueSummary)
}

func (s *issueSessionServiceImpl) UpdateStoryPoints(ctx context.Context, issueKey string, storyPoints float64, assigneeEmail, issueSummary string) error {
	return s.repo.UpdateStoryPoints(ctx, issueKey, storyPoints, assigneeEmail, issueSummary)
}

func (s *issueSessionServiceImpl) CloseSession(ctx context.Context, issueKey string, additionalSeconds int64) error {
	return s.repo.CloseSession(ctx, issueKey, additionalSeconds)
}

func (s *issueSessionServiceImpl) GetActiveSessions(ctx context.Context) ([]issue_session_dto.IssueSession, error) {
	return s.repo.GetActiveSessions(ctx)
}

func (s *issueSessionServiceImpl) GetSessionByIssueKey(ctx context.Context, issueKey string) (*issue_session_dto.IssueSession, bool, error) {
	return s.repo.GetSessionByIssueKey(ctx, issueKey)
}

func (s *issueSessionServiceImpl) UpdateLastOverageReminder(ctx context.Context, issueKey string) error {
	return s.repo.UpdateLastOverageReminder(ctx, issueKey)
}
