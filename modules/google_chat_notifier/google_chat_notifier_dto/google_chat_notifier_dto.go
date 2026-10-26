package google_chat_notifier_dto

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
}
