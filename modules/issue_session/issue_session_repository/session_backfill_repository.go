package issue_session_repository

import (
	"context"
	"database/sql"

	"nudge/modules/issue_session/issue_session_dto"
)

// SessionBackfillRepository writes the closed sessions rebuilt from Jira's
// changelog (Tech Spec "Active Sprint Tracking & Changelog Backfill").
// Kept apart from IssueSessionRepository so its many test fakes stay as-is.
type SessionBackfillRepository interface {
	// SessionExists reports whether issueKey already has a tr_issuesession row.
	SessionExists(ctx context.Context, issueKey string) (bool, error)
	// InsertClosedSession stores a closed (flagActive = 0) session; an
	// existing row for the same issue is never touched (added = false).
	InsertClosedSession(ctx context.Context, s issue_session_dto.IssueSession) (added bool, err error)
	// UpdateAccumulatedSeconds rewrites the time of past runs after a leave
	// change (Tech Spec "Papan Tiket & Cuti", AC4.2); the running period and
	// flagActive are left as they are.
	UpdateAccumulatedSeconds(ctx context.Context, issueKey string, seconds int64) error
}

// BackfillCreatedBy marks sessions written by the backfill.
const BackfillCreatedBy = "backfill"

// LeaveRecalcEditedBy marks sessions rewritten by the leave recalculation.
const LeaveRecalcEditedBy = "leave-recalc"

type sessionBackfillRepositoryImpl struct{ db *sql.DB }

func NewSessionBackfillRepository(db *sql.DB) SessionBackfillRepository {
	return &sessionBackfillRepositoryImpl{db: db}
}

func (r *sessionBackfillRepositoryImpl) SessionExists(ctx context.Context, issueKey string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tr_issuesession WHERE issueKey = ?`, issueKey).Scan(&n)
	return n > 0, err
}

func (r *sessionBackfillRepositoryImpl) InsertClosedSession(ctx context.Context, s issue_session_dto.IssueSession) (bool, error) {
	// INSERT IGNORE on uq_tr_issuesession_issuekey: if the poller opened a
	// session in the meantime, that row wins.
	const query = `
		INSERT IGNORE INTO tr_issuesession (
			issueKey, projectKey, storyPoints,
			accumulatedSeconds, currentSessionStartedDate, flagActive,
			assigneeEmail, issueSummary, sprintName, boardID, sprintID,
			createdBy, createdDate
		)
		VALUES (?, ?, ?, ?, NULL, 0, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`
	var sprintName sql.NullString
	if s.SprintName != "" {
		sprintName = sql.NullString{String: s.SprintName, Valid: true}
	}
	res, err := r.db.ExecContext(ctx, query,
		s.IssueKey, s.ProjectKey, s.StoryPoints, s.AccumulatedSeconds,
		s.AssigneeEmail, s.IssueSummary, sprintName, s.BoardID, s.SprintID, BackfillCreatedBy,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *sessionBackfillRepositoryImpl) UpdateAccumulatedSeconds(ctx context.Context, issueKey string, seconds int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tr_issuesession
		SET accumulatedSeconds = ?, editedBy = ?, editedDate = CURRENT_TIMESTAMP
		WHERE issueKey = ?`, seconds, LeaveRecalcEditedBy, issueKey)
	return err
}
