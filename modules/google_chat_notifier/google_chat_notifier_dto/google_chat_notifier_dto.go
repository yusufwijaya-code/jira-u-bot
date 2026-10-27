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
	// AssigneeEmail is the join key chat_dm_notifier_service uses to
	// resolve/cache a DM target — unused by the webhook notifier, which
	// always posts to the one shared space regardless of assignee.
	AssigneeEmail string
	// IsEarlyCompletion flags a Chapter 6 "finished early" notice — a
	// third message shape alongside tier and overage, reusing the exact
	// same routing (reminder_engine_service.pickNotifier) and formatting
	// (BuildMessageText) pipeline as both of those.
	IsEarlyCompletion bool
	// Deadline is the real clock time the ticket must be finished by,
	// projected forward through business hours from the moment the
	// reminder is evaluated. Zero-value (unset) for overage messages —
	// the deadline has already passed, so "must finish by" no longer
	// applies there.
	Deadline time.Time
	// CodeProcessStartedAt is when the ticket's current Code Process
	// session began (issue_session_dto.IssueSession.CurrentSessionStartedDate)
	// — shown on every message so the recipient can see the calculation's
	// starting point, not just its elapsed/remaining output.
	CodeProcessStartedAt time.Time
	// CodeReviewEnteredAt is when the ticket left Code Process for Code
	// Review — only set for IsEarlyCompletion messages, where it's the
	// counterpart to CodeProcessStartedAt (how long the ticket actually
	// spent in Code Process vs. its budget).
	CodeReviewEnteredAt time.Time
}
