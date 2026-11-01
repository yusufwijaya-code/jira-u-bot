package sprint_report_dto

// Row is one ticket's line in the report table — mirrors the columns in
// the PDF this feature reproduces (Cascading_KR_Estimated_vs_Actual_Time):
// Sprint, Jira Key, Task, Est. (h), Actual (h), Deviation, Result.
type Row struct {
	SprintName       string
	IssueKey         string
	Task             string
	EstimatedHours   float64
	ActualHours      float64
	DeviationPercent float64
	Within           bool
}

// Report is the full generated report for one or more selected sprints
// (Tech Spec "Dashboard Jira Manage", Chapter 11b, AC7.2).
type Report struct {
	Rows                      []Row
	SumEstimatedHours         float64
	SumActualHours            float64
	AggregateDeviationPercent float64
	AchievementPercent        float64
	CapPercent                float64
	WithinCount               int
	TotalCount                int
}
