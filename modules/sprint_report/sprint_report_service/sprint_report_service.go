package sprint_report_service

import (
	"context"

	"jira-u-bot/modules/sprint_report/sprint_report_dto"
)

// SprintReportService implements Tech Spec "Dashboard Jira Manage",
// Chapter 11b.
type SprintReportService interface {
	// ListAvailableSprints populates the sprint picker (spec AC7.1).
	ListAvailableSprints(ctx context.Context) ([]string, error)
	// GenerateReport computes the full report for sprintNames — an empty
	// capPercent means caller wants the default (handled by the caller,
	// not this method, so unit tests always pass an explicit cap).
	GenerateReport(ctx context.Context, sprintNames []string, capPercent float64) (sprint_report_dto.Report, error)
}
