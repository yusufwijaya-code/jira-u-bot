// Package tracking_rule manages ms_tracking_rule (Tech Spec "Aturan Board
// (Admin CRUD)"): admin CRUD validated against Jira, and loading the active
// rules into the jira_client_dto registry every tracking module reads.
package tracking_rule

import (
	"errors"
	"time"

	"nudge/modules/jira_client/jira_client_dto"
)

// Rule is one ms_tracking_rule row.
type Rule struct {
	ProjectKey                  string
	ProjectName                 string
	TrackedStatuses             []string
	DoneStatus                  string
	ReviewStatus                string // "" = none (NULL)
	StoryPointFields            []string
	StoryPointWriteField        string
	StoryPointWriteFieldSubtask string
	IsActive                    bool
	EditedBy                    string
	EditedDate                  time.Time
	// AvatarBoardID: a board of this project with a cached avatar (0 = none).
	AvatarBoardID int64
}

// Tracking is the rule as the registry holds it.
func (r Rule) Tracking() jira_client_dto.TrackingRule {
	return jira_client_dto.TrackingRule{
		ProjectKey:                  r.ProjectKey,
		TrackedStatuses:             r.TrackedStatuses,
		DoneStatus:                  r.DoneStatus,
		ReviewStatus:                r.ReviewStatus,
		StoryPointFields:            r.StoryPointFields,
		StoryPointWriteField:        r.StoryPointWriteField,
		StoryPointWriteFieldSubtask: r.StoryPointWriteFieldSubtask,
	}
}

// FieldRef is a Story Point field with its Jira name.
type FieldRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RuleView is one rule in the API (GET list item, POST/PUT response).
type RuleView struct {
	ProjectKey                  string     `json:"projectKey"`
	ProjectName                 string     `json:"projectName"`
	TrackedStatuses             []string   `json:"trackedStatuses"`
	DoneStatus                  string     `json:"doneStatus"`
	ReviewStatus                *string    `json:"reviewStatus"`
	StoryPointFields            []FieldRef `json:"storyPointFields"`
	StoryPointWriteField        string     `json:"storyPointWriteField"`
	StoryPointWriteFieldSubtask string     `json:"storyPointWriteFieldSubtask"`
	IsActive                    bool       `json:"isActive"`
	EditedBy                    string     `json:"editedBy"`
	EditedDate                  time.Time  `json:"editedDate"`
	// BoardID: serve the project picture via GET /api/dashboard/boards/:boardId/avatar (null = no picture).
	BoardID *int64 `json:"boardId"`
}

// RuleInput is the POST/PUT body (projectKey is ignored on PUT).
type RuleInput struct {
	ProjectKey                  string   `json:"projectKey"`
	TrackedStatuses             []string `json:"trackedStatuses"`
	DoneStatus                  string   `json:"doneStatus"`
	ReviewStatus                *string  `json:"reviewStatus"`
	StoryPointFields            []string `json:"storyPointFields"`
	StoryPointWriteField        string   `json:"storyPointWriteField"`
	StoryPointWriteFieldSubtask string   `json:"storyPointWriteFieldSubtask"`
}

// Lookup is what the form picks from: GET /admin/tracking-rules/jira/:projectKey.
type Lookup struct {
	ProjectKey   string                        `json:"projectKey"`
	ProjectName  string                        `json:"projectName"`
	AvatarURL    string                        `json:"avatarUrl"`
	Statuses     []string                      `json:"statuses"`
	NumberFields []jira_client_dto.NumberField `json:"numberFields"`
}

var (
	ErrInvalidProjectKey = errors.New("tracking_rule: invalid project key")
	ErrProjectNotFound   = errors.New("tracking_rule: project not found in jira")
	ErrJiraUnavailable   = errors.New("tracking_rule: jira unavailable")
	ErrRuleExists        = errors.New("tracking_rule: rule exists")
	ErrRuleNotFound      = errors.New("tracking_rule: rule not found")
	ErrLastActiveRule    = errors.New("tracking_rule: last active rule")
	ErrAlreadyRunning    = errors.New("tracking_rule: recalculation already running")
	ErrBoardNotFound     = errors.New("tracking_rule: board not found")
)

// ValidationError carries one message per invalid input (400 validation_failed).
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "tracking_rule: validation failed" }
