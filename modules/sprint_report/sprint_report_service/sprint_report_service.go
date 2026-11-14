package sprint_report_service

import (
	"context"

	"nudge/modules/sprint_report/sprint_report_dto"
)

// SprintReportService implements Tech Spec "Dashboard Jira Manage",
// Chapter 11b.
type SprintReportService interface {
	// ListAvailableSprints populates the sprint picker (spec AC7.1) with
	// only the sprints in which email themselves has tracked tickets.
	ListAvailableSprints(ctx context.Context, email string) ([]string, error)
	// GenerateReport computes the report for sprintNames over email's OWN
	// tickets only — other users' sessions never enter the report. An empty
	// capPercent means caller wants the default (handled by the caller,
	// not this method, so unit tests always pass an explicit cap).
	GenerateReport(ctx context.Context, email string, sprintNames []string, capPercent float64) (sprint_report_dto.Report, error)
}
