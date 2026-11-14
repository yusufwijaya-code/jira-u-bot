package user_dashboard_service

import (
	"context"

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
	// GetTeamView returns email's own active (Code Process) tickets.
	GetTeamView(ctx context.Context, email string) (user_dashboard_dto.TeamView, error)
	// GetOverdueView returns email's own active tickets whose elapsed
	// time has passed their budget (spec AC3.4).
	GetOverdueView(ctx context.Context, email string) ([]user_dashboard_dto.OverdueItem, error)
}
