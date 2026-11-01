package user_dashboard_dto

// Card is one ticket as shown in the dashboard — BudgetDetail fields are
// only populated for tickets currently in Code Process with a tracked
// tr_issuesession row (Tech Spec "Dashboard Jira Manage", AC3.2); every
// other status shows just IssueKey/Summary/Status.
type Card struct {
	IssueKey        string
	Summary         string
	Status          string
	HasBudgetDetail bool
	StoryPoints     float64
	ElapsedLabel    string
	RemainingLabel  string
	DeadlineLabel   string
}

// BoardColumn groups Card by Jira status, in first-seen order from the
// Jira API response (spec AC3.1 — no workflow-aware ordering, see Out of
// Scope).
type BoardColumn struct {
	Status string
	Cards  []Card
}

type Board struct {
	Columns []BoardColumn
}

// TeamGroup is one assignee's active (Code Process) tickets — Chapter 9c,
// equivalent to the bot's /u-team.
type TeamGroup struct {
	AssigneeEmail string
	Cards         []Card
}

type TeamView struct {
	Groups []TeamGroup
}

// OverdueItem is one active ticket whose elapsed time has passed its
// budget — Chapter 9c, equivalent to the bot's /u-overdue.
type OverdueItem struct {
	Card
	AssigneeEmail string
}
