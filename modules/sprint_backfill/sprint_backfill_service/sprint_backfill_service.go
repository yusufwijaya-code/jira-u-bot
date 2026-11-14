package sprint_backfill_service

import "context"

// SprintBackfillService rebuilds the time of active-sprint tickets the
// poller never saw (bot down while they were worked on) from Jira's
// changelog and stores it as closed sessions (Tech Spec "Active Sprint
// Tracking & Changelog Backfill").
type SprintBackfillService interface {
	// RunForEmail backfills one user's active-sprint tickets (the Sprint
	// card's "Hitung ulang dari Jira" button). Returns how many sessions
	// were added.
	RunForEmail(ctx context.Context, email string) (added int, err error)
	// RunAll backfills every registered, active user; per-user failures are
	// logged and skipped.
	RunAll(ctx context.Context) (added int)
	// RunDaily runs RunAll once now and then every day at 07:00 WIB until
	// ctx is cancelled.
	RunDaily(ctx context.Context)
}
