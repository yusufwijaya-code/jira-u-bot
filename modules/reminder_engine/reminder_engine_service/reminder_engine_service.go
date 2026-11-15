package reminder_engine_service

import (
	"context"

	"nudge/modules/issue_session/issue_session_dto"
	"nudge/modules/jira_client/jira_client_dto"
)

type ReminderEngineService interface {
	// ApplicableTiers returns, largest-first, the tiers whose duration is
	// strictly shorter than budgetSeconds (spec AC2.2 — a tier that would
	// fall at or before the ticket's entire budget never fires).
	ApplicableTiers(budgetSeconds int64) []string
	EvaluateAndSend(ctx context.Context, issue jira_client_dto.Issue, session issue_session_dto.IssueSession) error
	// Status-change notices (finished early, back to To Do, ...) moved to
	// status_watch (Tech Spec "Notifikasi Perubahan Status Tiket").
}
