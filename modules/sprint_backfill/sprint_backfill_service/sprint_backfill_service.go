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
	// RecalculateForEmail rewrites accumulatedSeconds of email's existing
	// active-sprint sessions from Jira's changelog with their current
	// calendar (holidays + leave) — Tech Spec "Papan Tiket & Cuti", AC4.2.
	// Only finished runs are counted; the running period stays with the
	// poller. Returns how many sessions were rewritten.
	RecalculateForEmail(ctx context.Context, email string) (updated int, err error)
}
