package google_chat_notifier_dto

import "time"

type ReminderMessage struct {
	IssueKey       string
	IssueURL       string
	IssueSummary   string
	AssigneeName   string
	StoryPoints    float64
	BudgetSeconds  int64
	ElapsedSeconds int64
	Tier           string
	IsOverage      bool
	// Deadline is the real clock time the ticket must be finished by,
	// projected forward through business hours from the moment the
	// reminder is evaluated. Zero-value (unset) for overage messages —
	// the deadline has already passed, so "must finish by" no longer
	// applies there.
	Deadline time.Time
}
