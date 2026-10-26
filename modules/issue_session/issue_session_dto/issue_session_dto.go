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
	CreatedBy                 string
	CreatedDate               time.Time
}
