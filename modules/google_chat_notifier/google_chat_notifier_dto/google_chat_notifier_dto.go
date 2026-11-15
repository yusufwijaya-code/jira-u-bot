package google_chat_notifier_dto

import "time"

type ReminderMessage struct {
	// Text, when set, is sent as-is instead of the reminder layout (status
	// change notices, Tech Spec "Notifikasi Perubahan Status Tiket").
	Text           string
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
	// InReview flags a reminder for a ticket that is in its project's
	// tracked code-review status (CORE "CODE REVIEW") — review time still
	// counts toward the budget there.
	InReview bool
}
