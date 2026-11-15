package status_watch_service

import "context"

// StatusWatchService DMs the assignee whenever one of their active-sprint
// tickets changes status — moved in Jira or in Nudge (Tech Spec
// "Notifikasi Perubahan Status Tiket").
type StatusWatchService interface {
	// RunOnce checks Jira once; the cursor only advances on success.
	RunOnce(ctx context.Context) error
	// Run checks every 10 seconds and drops stale snapshots daily, until
	// ctx is cancelled.
	Run(ctx context.Context)
}
