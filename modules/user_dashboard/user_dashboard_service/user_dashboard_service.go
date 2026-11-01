package user_dashboard_service

import (
	"context"

	"jira-u-bot/modules/user_dashboard/user_dashboard_dto"
)

// UserDashboardService implements Tech Spec "Dashboard Jira Manage",
// Chapter 9a (reminder preference), 9b (My Board), 9c (Team/Overdue).
type UserDashboardService interface {
	// GetMyBoard returns every OMS/CORE ticket assigned to email,
	// regardless of status, grouped by status (spec AC3.1/AC3.2).
	GetMyBoard(ctx context.Context, email string) (user_dashboard_dto.Board, error)
	// GetTeamView mirrors /u-team — all active (Code Process) tickets
	// grouped by assignee, open to every registered user (spec AC3.3).
	GetTeamView(ctx context.Context) (user_dashboard_dto.TeamView, error)
	// GetOverdueView mirrors /u-overdue — active tickets whose elapsed
	// time has passed their budget (spec AC3.4).
	GetOverdueView(ctx context.Context) ([]user_dashboard_dto.OverdueItem, error)
	// SetReminderTierPreference persists email's chosen subset of the 6
	// reminder tiers (spec AC2.1) — csv may be "" (deliberately none).
	SetReminderTierPreference(ctx context.Context, email, csv string) error
	// GetReminderTierPreference returns email's saved preference CSV, or
	// nil if they've never set one (spec AC2.2 default) — used to
	// pre-check the right boxes when the dashboard page renders.
	GetReminderTierPreference(ctx context.Context, email string) (*string, error)
}
