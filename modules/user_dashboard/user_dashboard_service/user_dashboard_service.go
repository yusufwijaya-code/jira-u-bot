package user_dashboard_service

import (
	"context"

	"nudge/modules/move_fields"
	"nudge/modules/user_dashboard/user_dashboard_dto"
)

// UserDashboardService implements Tech Spec "Dashboard Jira Manage",
// Chapter 9b (My Board) and 9c (Team/Overdue). Reminder preferences moved to
// the user_setting module.
//
// Every read method takes the caller's email and returns ONLY that user's
// own tickets - callers pass the session's email, never a request param, and
// there is no admin exception.
type UserDashboardService interface {
	// GetMyBoard returns email's tickets in ACTIVE sprints, grouped by
	// status — only boardID's active sprint when boardID > 0, every active
	// sprint when 0 (Tech Spec "Per-Board Settings & Active Sprint
	// Dashboard", AC2.1).
	GetMyBoard(ctx context.Context, email string, boardID int64) (user_dashboard_dto.Board, error)
	// ListBoards returns the boards whose active sprint holds tickets
	// assigned to email, ordered by name (AC1.1).
	ListBoards(ctx context.Context, email string) ([]user_dashboard_dto.BoardSummary, error)
	// DescribeBoard returns a board's name and project key from the board
	// cache — "Board #<id>" and "" when it can't be looked up. Never fails.
	DescribeBoard(ctx context.Context, boardID int64) (name, projectKey string)
	// GetTeamView returns email's own active (Code Process) tickets.
	GetTeamView(ctx context.Context, email string) (user_dashboard_dto.TeamView, error)
	// GetOverdueView returns email's own active tickets whose elapsed
	// time has passed their budget (spec AC3.4).
	GetOverdueView(ctx context.Context, email string) ([]user_dashboard_dto.OverdueItem, error)
	// ListTransitions returns the moves issueKey's Jira workflow allows from
	// its current status — CORE and OMS differ, so the list always comes
	// from Jira (Tech Spec "Papan Tiket & Cuti", AC1.3). ErrNotOwnTicket
	// unless issueKey is one of email's active-sprint tickets.
	ListTransitions(ctx context.Context, email, issueKey string) (user_dashboard_dto.TicketTransitions, error)
	// Transition executes transitionID on issueKey as email and returns the
	// new status; ErrTransitionUnavailable when the workflow no longer
	// offers it, ErrNotOwnTicket as in ListTransitions.
	// A *move_fields.FieldsRequiredError when the move needs fields filled
	// first (the ticket is not moved).
	Transition(ctx context.Context, email, issueKey, transitionID string) (newStatus string, err error)
	// MoveRequirements: the still-empty fields transitionID requires.
	MoveRequirements(ctx context.Context, email, issueKey, transitionID string) (move_fields.Required, error)
	// TransitionWithFields fills values, then moves (see move_fields.Service.Apply).
	TransitionWithFields(ctx context.Context, email, issueKey, transitionID string, values map[string]any) (newStatus string, err error)
}
