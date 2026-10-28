package reminder_engine_service

import (
	"context"
	"time"

	"jira-u-bot/modules/issue_session/issue_session_dto"
	"jira-u-bot/modules/jira_client/jira_client_dto"
)

type ReminderEngineService interface {
	// ApplicableTiers returns, largest-first, the tiers whose duration is
	// strictly shorter than budgetSeconds (spec AC2.2 — a tier that would
	// fall at or before the ticket's entire budget never fires).
	ApplicableTiers(budgetSeconds int64) []string
	EvaluateAndSend(ctx context.Context, issue jira_client_dto.Issue, session issue_session_dto.IssueSession) error
	// NotifyIfEarlyCompletion implements spec AC4.1/AC4.2/AC4.3: a ticket
	// that left "Code Process" with time to spare (elapsed < budget) gets
	// a one-off notice showing the unused time and what the deadline would
	// have been. The Code Review status check (AC4.5) happens in the
	// poller BEFORE this is called — by the time this method runs, that
	// decision has already been made. codeProcessStartedAt/codeReviewEnteredAt
	// let the message show both ends of the ticket's time in Code Process,
	// not just the elapsed/remaining totals.
	NotifyIfEarlyCompletion(ctx context.Context, issueKey, assigneeEmail, issueSummary string, elapsedSeconds, budgetSeconds int64, codeProcessStartedAt, codeReviewEnteredAt time.Time) error
	// NotifyReturnedToTodo sends a notice when a ticket leaves Code Process
	// back to To Do — whether via /u-geser or a manual Jira change. Unlike
	// NotifyIfEarlyCompletion, this always sends: there's no "early" or
	// "late" framing for simply being pulled back to To Do.
	NotifyReturnedToTodo(ctx context.Context, issueKey, assigneeEmail, issueSummary string, elapsedSeconds, budgetSeconds int64, codeProcessStartedAt time.Time) error
}
