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
	// GetMyBoard returns every OMS/CORE ticket assigned to email,
	// regardless of status, grouped by status (spec AC3.1/AC3.2).
	GetMyBoard(ctx context.Context, email string) (user_dashboard_dto.Board, error)
	// GetTeamView returns email's own active (Code Process) tickets.
	GetTeamView(ctx context.Context, email string) (user_dashboard_dto.TeamView, error)
	// GetOverdueView returns email's own active tickets whose elapsed
	// time has passed their budget (spec AC3.4).
	GetOverdueView(ctx context.Context, email string) ([]user_dashboard_dto.OverdueItem, error)
}
