package issue_session_dto

import "time"

type IssueSession struct {
	IssueSessionID            int64
	IssueKey                  string
	ProjectKey                string
	StoryPoints               float64
	AccumulatedSeconds        int64
	CurrentSessionStartedDate *time.Time
	FlagActive                bool
	LastOverageReminderDate   *time.Time
	AssigneeEmail             string
	IssueSummary              string
	// SprintName is set at session creation/update time from Jira's Sprint
	// field (Tech Spec "Dashboard Jira Manage", Chapter 11a) — empty for
	// any session opened before this field existed, never backfilled.
	SprintName  string
	CreatedBy   string
	CreatedDate time.Time
}
