package issue_session_repository

import (
	"context"
	"database/sql"
	"time"

	"jira-u-bot/modules/issue_session/issue_session_dto"
)

type issueSessionRepositoryImpl struct {
	db *sql.DB
}

func NewIssueSessionRepository(db *sql.DB) IssueSessionRepository {
	return &issueSessionRepositoryImpl{db: db}
}

func (r *issueSessionRepositoryImpl) OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, storyPoints float64, assigneeEmail, issueSummary string) error {
	const query = `
		INSERT INTO tr_issuesession (
			issueKey, projectKey, storyPoints,
			accumulatedSeconds, currentSessionStartedDate, flagActive,
			assigneeEmail, issueSummary,
			createdBy, createdDate
		)
		VALUES (?, ?, ?, 0, CURRENT_TIMESTAMP, 1, ?, ?, 'poller', CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE
			currentSessionStartedDate = CURRENT_TIMESTAMP,
			flagActive = 1,
			storyPoints = VALUES(storyPoints),
			assigneeEmail = VALUES(assigneeEmail),
			issueSummary = VALUES(issueSummary),
			editedBy = 'poller',
			editedDate = CURRENT_TIMESTAMP
	`
	_, err := r.db.ExecContext(ctx, query, issueKey, projectKey, storyPoints, assigneeEmail, issueSummary)
	return err
}

func (r *issueSessionRepositoryImpl) OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, storyPoints float64, startedAt time.Time, assigneeEmail, issueSummary string) error {
	const query = `
		INSERT INTO tr_issuesession (
			issueKey, projectKey, storyPoints,
			accumulatedSeconds, currentSessionStartedDate, flagActive,
			assigneeEmail, issueSummary,
			createdBy, createdDate
		)
		VALUES (?, ?, ?, 0, ?, 1, ?, ?, 'poller', CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE
			currentSessionStartedDate = VALUES(currentSessionStartedDate),
			flagActive = 1,
			storyPoints = VALUES(storyPoints),
			assigneeEmail = VALUES(assigneeEmail),
			issueSummary = VALUES(issueSummary),
			editedBy = 'poller',
			editedDate = CURRENT_TIMESTAMP
	`
	_, err := r.db.ExecContext(ctx, query, issueKey, projectKey, storyPoints, startedAt, assigneeEmail, issueSummary)
	return err
}

func (r *issueSessionRepositoryImpl) UpdateStoryPoints(ctx context.Context, issueKey string, storyPoints float64) error {
	const query = `
		UPDATE tr_issuesession
		SET storyPoints = ?,
		    editedBy = 'poller',
		    editedDate = CURRENT_TIMESTAMP
		WHERE issueKey = ?
	`
	_, err := r.db.ExecContext(ctx, query, storyPoints, issueKey)
	return err
}

func (r *issueSessionRepositoryImpl) CloseSession(ctx context.Context, issueKey string, additionalSeconds int64) error {
	const query = `
		UPDATE tr_issuesession
		SET accumulatedSeconds = accumulatedSeconds + ?,
		    currentSessionStartedDate = NULL,
		    flagActive = 0,
		    editedBy = 'poller',
		    editedDate = CURRENT_TIMESTAMP
		WHERE issueKey = ?
	`
	_, err := r.db.ExecContext(ctx, query, additionalSeconds, issueKey)
	return err
}

func (r *issueSessionRepositoryImpl) GetActiveSessions(ctx context.Context) ([]issue_session_dto.IssueSession, error) {
	const query = `
		SELECT issueSessionID, issueKey, projectKey, storyPoints,
		       accumulatedSeconds, currentSessionStartedDate, flagActive,
		       lastOverageReminderDate, assigneeEmail, issueSummary, createdBy, createdDate
		FROM tr_issuesession
		WHERE flagActive = 1
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []issue_session_dto.IssueSession
	for rows.Next() {
		s, err := scanIssueSessionRow(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (r *issueSessionRepositoryImpl) GetSessionByIssueKey(ctx context.Context, issueKey string) (*issue_session_dto.IssueSession, bool, error) {
	const query = `
		SELECT issueSessionID, issueKey, projectKey, storyPoints,
		       accumulatedSeconds, currentSessionStartedDate, flagActive,
		       lastOverageReminderDate, assigneeEmail, issueSummary, createdBy, createdDate
		FROM tr_issuesession
		WHERE issueKey = ?
	`
	row := r.db.QueryRowContext(ctx, query, issueKey)
	s, err := scanIssueSessionRow(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &s, true, nil
}

func (r *issueSessionRepositoryImpl) UpdateLastOverageReminder(ctx context.Context, issueKey string) error {
	const query = `
		UPDATE tr_issuesession
		SET lastOverageReminderDate = CURRENT_TIMESTAMP,
		    editedBy = 'poller',
		    editedDate = CURRENT_TIMESTAMP
		WHERE issueKey = ?
	`
	_, err := r.db.ExecContext(ctx, query, issueKey)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanIssueSessionRow(scanner rowScanner) (issue_session_dto.IssueSession, error) {
	var s issue_session_dto.IssueSession
	var currentSessionStartedDate sql.NullTime
	var lastOverageReminderDate sql.NullTime
	var assigneeEmail, issueSummary sql.NullString

	err := scanner.Scan(
		&s.IssueSessionID, &s.IssueKey, &s.ProjectKey, &s.StoryPoints,
		&s.AccumulatedSeconds, &currentSessionStartedDate, &s.FlagActive,
		&lastOverageReminderDate, &assigneeEmail, &issueSummary, &s.CreatedBy, &s.CreatedDate,
	)
	if err != nil {
		return issue_session_dto.IssueSession{}, err
	}

	if currentSessionStartedDate.Valid {
		t := currentSessionStartedDate.Time
		s.CurrentSessionStartedDate = &t
	}
	if lastOverageReminderDate.Valid {
		t := lastOverageReminderDate.Time
		s.LastOverageReminderDate = &t
	}
	s.AssigneeEmail = assigneeEmail.String
	s.IssueSummary = issueSummary.String

	return s, nil
}
