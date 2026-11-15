package sprint_report_service

import (
	"context"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/issue_session/issue_session_repository"
	"nudge/modules/issue_session/issue_session_service"
	"nudge/modules/sprint_report/sprint_report_dto"
)

type sprintReportServiceImpl struct {
	issueSessionRepo issue_session_repository.IssueSessionRepository
	businessHours    business_hours_service.BusinessHoursService
	holidayCalendar  business_hours_dto.CalendarSource
}

func NewSprintReportService(
	issueSessionRepo issue_session_repository.IssueSessionRepository,
	businessHours business_hours_service.BusinessHoursService,
	holidayCalendar business_hours_dto.CalendarSource,
) SprintReportService {
	return &sprintReportServiceImpl{
		issueSessionRepo: issueSessionRepo,
		businessHours:    businessHours,
		holidayCalendar:  holidayCalendar,
	}
}

// ListAvailableSprints narrows the global sprint list down to the sprints
// where email has at least one tracked ticket — a user never even sees the
// names of sprints they have no tickets in.
func (s *sprintReportServiceImpl) ListAvailableSprints(ctx context.Context, email string) ([]string, error) {
	all, err := s.issueSessionRepo.ListDistinctSprintNames(ctx)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, nil
	}
	sessions, err := s.issueSessionRepo.FindBySprintNames(ctx, all)
	if err != nil {
		return nil, err
	}
	owned := make(map[string]bool, len(all))
	for _, session := range sessions {
		if session.AssigneeEmail == email {
			owned[session.SprintName] = true
		}
	}
	var available []string
	for _, name := range all {
		if owned[name] {
			available = append(available, name)
		}
	}
	return available, nil
}

func (s *sprintReportServiceImpl) GenerateReport(ctx context.Context, email string, sprintNames []string, capPercent float64) (sprint_report_dto.Report, error) {
	sessions, err := s.issueSessionRepo.FindBySprintNames(ctx, sprintNames)
	if err != nil {
		return sprint_report_dto.Report{}, err
	}

	report := sprint_report_dto.Report{CapPercent: capPercent}
	for _, session := range sessions {
		if session.AssigneeEmail != email {
			continue // own tickets only, regardless of who else is in the sprint
		}
		estimatedHours := session.StoryPoints
		// AC7.5: live elapsed via CalculateElapsedSeconds, not raw
		// accumulatedSeconds — a still-active session's actual time stays
		// accurate as of the moment the report is generated.
		actualHours := float64(issue_session_service.CalculateElapsedSeconds(session, s.businessHours, s.holidayCalendar.ForEmail(ctx, session.AssigneeEmail))) / 3600.0

		row := sprint_report_dto.Row{
			SprintName:     session.SprintName,
			IssueKey:       session.IssueKey,
			Task:           session.IssueSummary,
			EstimatedHours: estimatedHours,
			ActualHours:    actualHours,
		}
		row.DeviationPercent = deviationPercent(estimatedHours, actualHours)
		row.Within = row.DeviationPercent <= capPercent

		report.Rows = append(report.Rows, row)
		report.SumEstimatedHours += estimatedHours
		report.SumActualHours += actualHours
		report.TotalCount++
		if row.Within {
			report.WithinCount++
		}
	}

	report.AggregateDeviationPercent = deviationPercent(report.SumEstimatedHours, report.SumActualHours)
	report.AchievementPercent = achievementPercent(capPercent, report.AggregateDeviationPercent)
	return report, nil
}

// deviationPercent is |Actual - Estimated| / Estimated * 100 (spec AC7.2
// formula) — estimated is guaranteed > 0 for any real tracked session
// (poller_service.resolveEligibility rejects storyPoints <= 0 before a
// session is ever created), so this guard is purely defensive.
func deviationPercent(estimated, actual float64) float64 {
	if estimated == 0 {
		return 0
	}
	diff := actual - estimated
	if diff < 0 {
		diff = -diff
	}
	return diff / estimated * 100
}

// achievementPercent is (cap / deviation) * 100 (spec AC7.2 formula) — an
// aggregate deviation of exactly 0% (estimated matches actual to the
// second) is treated as the ceiling case rather than computing a
// division-by-zero: a perfect match is at least as good as hitting the
// cap exactly. Anything within the cap is likewise 100%: without that
// ceiling a tiny deviation showed absurd figures like 11,617%.
func achievementPercent(capPercent, deviationPercent float64) float64 {
	if deviationPercent <= capPercent {
		return 100
	}
	return capPercent / deviationPercent * 100
}
