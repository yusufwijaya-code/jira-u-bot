package user_dashboard_dto

// Card is one ticket as shown in the dashboard — BudgetDetail fields are
// only populated for tickets currently in Code Process with a tracked
// tr_issuesession row (Tech Spec "Dashboard Jira Manage", AC3.2); every
// other status shows just IssueKey/Summary/Status.
type Card struct {
	IssueKey        string  `json:"issueKey"`
	Summary         string  `json:"summary"`
	Status          string  `json:"status"`
	HasBudgetDetail bool    `json:"hasBudgetDetail"`
	StoryPoints     float64 `json:"storyPoints"`
	ElapsedLabel    string  `json:"elapsedLabel"`
	RemainingLabel  string  `json:"remainingLabel"`
	DeadlineLabel   string  `json:"deadlineLabel"`
	// ElapsedSeconds / BudgetSeconds back the dashboard's budget progress
	// bar; only set when HasBudgetDetail.
	ElapsedSeconds int64 `json:"elapsedSeconds"`
	BudgetSeconds  int64 `json:"budgetSeconds"`
	// StartedAt is when the current tracked session started (entered Code
	// Process); DeadlineAt is when the budget runs out in business hours.
	// RFC3339, only set when HasBudgetDetail — the dashboard formats them.
	StartedAt  string `json:"startedAt,omitempty"`
	DeadlineAt string `json:"deadlineAt,omitempty"`
	// IssueURL links the card to the ticket in Jira.
	IssueURL string `json:"issueURL"`
	// BoardID is the Jira Agile board of the ticket's active sprint (0 =
	// not in an active sprint).
	BoardID int64 `json:"boardId"`
	// Parent* / Relation are set on a branch (Tech Spec "Subtask & Linked
	// Item Branches"): a subtask or linked item hanging under one of the
	// caller's active-sprint tickets. Relation is "subtask" or the Jira link
	// wording ("relates to", "blocks", ...).
	ParentKey     string `json:"parentKey,omitempty"`
	ParentSummary string `json:"parentSummary,omitempty"`
	ParentStatus  string `json:"parentStatus,omitempty"`
	Relation      string `json:"relation,omitempty"`
}

// SprintSummary is a board's active sprint.
type SprintSummary struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// BoardSummary is one entry of the board picker: a board whose active
// sprint holds tickets assigned to the caller (Tech Spec "Per-Board
// Settings & Active Sprint Dashboard", AC1.1).
type BoardSummary struct {
	BoardID      int64         `json:"boardId"`
	Name         string        `json:"name"`
	ProjectKey   string        `json:"projectKey"`
	ActiveSprint SprintSummary `json:"activeSprint"`
	TicketCount  int           `json:"ticketCount"`
	// ReviewTracked: this board's project counts code review time (CORE) —
	// the dashboard shows a "Sedang di-review" card for it.
	ReviewTracked bool `json:"reviewTracked"`
}

// BoardColumn groups Card by Jira status, in first-seen order from the
// Jira API response (spec AC3.1 — no workflow-aware ordering, see Out of
// Scope).
type BoardColumn struct {
	Status string `json:"status"`
	Cards  []Card `json:"cards"`
}

type Board struct {
	Columns []BoardColumn `json:"columns"`
	// Related are the caller's subtasks/linked items (tracked status) of
	// their active-sprint tickets that aren't in the sprint themselves —
	// only for the "Sedang dikerjakan"/"Sedang di-review" cards, never
	// counted in Columns.
	Related []Card `json:"related"`
}

// TeamGroup is one assignee's active (Code Process) tickets — Chapter 9c,
// equivalent to the bot's /u-team.
type TeamGroup struct {
	AssigneeEmail string `json:"assigneeEmail"`
	Cards         []Card `json:"cards"`
}

type TeamView struct {
	Groups []TeamGroup `json:"groups"`
}

// OverdueItem is one active ticket whose elapsed time has passed its
// budget — Chapter 9c, equivalent to the bot's /u-overdue.
type OverdueItem struct {
	Card
	AssigneeEmail string `json:"assigneeEmail"`
}
