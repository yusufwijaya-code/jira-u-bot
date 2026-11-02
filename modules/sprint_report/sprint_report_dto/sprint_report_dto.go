package sprint_report_dto

// Row is one ticket's line in the report table — mirrors the columns in
// the PDF this feature reproduces (Cascading_KR_Estimated_vs_Actual_Time):
// Sprint, Jira Key, Task, Est. (h), Actual (h), Deviation, Result.
type Row struct {
	SprintName       string  `json:"sprintName"`
	IssueKey         string  `json:"issueKey"`
	Task             string  `json:"task"`
	EstimatedHours   float64 `json:"estimatedHours"`
	ActualHours      float64 `json:"actualHours"`
	DeviationPercent float64 `json:"deviationPercent"`
	Within           bool    `json:"within"`
}

// Report is the full generated report for one or more selected sprints
// (Tech Spec "Dashboard Jira Manage", Chapter 11b, AC7.2).
type Report struct {
	Rows                      []Row   `json:"rows"`
	SumEstimatedHours         float64 `json:"sumEstimatedHours"`
	SumActualHours            float64 `json:"sumActualHours"`
	AggregateDeviationPercent float64 `json:"aggregateDeviationPercent"`
	AchievementPercent        float64 `json:"achievementPercent"`
	CapPercent                float64 `json:"capPercent"`
	WithinCount               int     `json:"withinCount"`
	TotalCount                int     `json:"totalCount"`
}
