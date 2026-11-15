package sprint_report_service

import (
	"context"
	"math"
	"testing"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/issue_session/issue_session_dto"
)

type fakeIssueSessionRepoForReport struct {
	sessions            []issue_session_dto.IssueSession
	distinctSprintNames []string
}

func (f *fakeIssueSessionRepoForReport) OpenOrResumeSession(ctx context.Context, issueKey, projectKey string, sp float64, assigneeEmail, issueSummary, sprintName string) error {
	return nil
}
func (f *fakeIssueSessionRepoForReport) OpenOrResumeSessionAt(ctx context.Context, issueKey, projectKey string, sp float64, startedAt time.Time, assigneeEmail, issueSummary, sprintName string) error {
	return nil
}
func (f *fakeIssueSessionRepoForReport) UpdateStoryPoints(ctx context.Context, issueKey string, sp float64, assigneeEmail, issueSummary, sprintName string) error {
	return nil
}
func (f *fakeIssueSessionRepoForReport) CloseSession(ctx context.Context, issueKey string, additionalSeconds int64) error {
	return nil
}
func (f *fakeIssueSessionRepoForReport) GetActiveSessions(ctx context.Context) ([]issue_session_dto.IssueSession, error) {
	return nil, nil
}
func (f *fakeIssueSessionRepoForReport) GetSessionByIssueKey(ctx context.Context, issueKey string) (*issue_session_dto.IssueSession, bool, error) {
	return nil, false, nil
}
func (f *fakeIssueSessionRepoForReport) UpdateLastOverageReminder(ctx context.Context, issueKey string) error {
	return nil
}
func (f *fakeIssueSessionRepoForReport) FindActiveByAssigneeEmail(ctx context.Context, email string) ([]issue_session_dto.IssueSession, error) {
	return nil, nil
}
func (f *fakeIssueSessionRepoForReport) FindBySprintNames(ctx context.Context, sprintNames []string) ([]issue_session_dto.IssueSession, error) {
	return f.sessions, nil
}
func (f *fakeIssueSessionRepoForReport) ListDistinctSprintNames(ctx context.Context) ([]string, error) {
	return f.distinctSprintNames, nil
}

func newTestReportService(repo *fakeIssueSessionRepoForReport) SprintReportService {
	return NewSprintReportService(repo, business_hours_service.NewBusinessHoursService(), business_hours_dto.HolidayCalendar{})
}

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestGenerateReport_MatchesRealPDFFixtureRow(t *testing.T) {
	// OMS-11035 from Cascading_KR_Estimated_vs_Actual_Time...pdf: Est 7.00h,
	// Actual 5.44h, Deviation -22.29%, Result OUT (cap 12%).
	// 5.44h = 19584s, AccumulatedSeconds (not CurrentSessionStartedDate) so
	// the test is wall-clock independent.
	repo := &fakeIssueSessionRepoForReport{sessions: []issue_session_dto.IssueSession{
		{AssigneeEmail: "dev@esb.co.id", IssueKey: "OMS-11035", IssueSummary: "LHDN Setting API", SprintName: "Sprint 15", StoryPoints: 7.0, AccumulatedSeconds: 19584},
	}}
	svc := newTestReportService(repo)

	report, err := svc.GenerateReport(context.Background(), "dev@esb.co.id", []string{"Sprint 15"}, 12)
	if err != nil {
		t.Fatalf("GenerateReport returned error: %v", err)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("len(Rows) = %d, want 1", len(report.Rows))
	}
	row := report.Rows[0]
	if !almostEqual(row.ActualHours, 5.44) {
		t.Errorf("ActualHours = %.4f, want ~5.44", row.ActualHours)
	}
	if !almostEqual(row.DeviationPercent, 22.29) {
		t.Errorf("DeviationPercent = %.4f, want ~22.29", row.DeviationPercent)
	}
	if row.Within {
		t.Error("Within = true, want false — 22.29%% exceeds the 12%% cap (Result OUT)")
	}
}

func TestGenerateReport_AggregateMatchesRealPDFFixtureTotals(t *testing.T) {
	// From the PDF's own totals: Sum Estimate 401.30h, Sum Actual 503.42h,
	// Aggregate Deviation 25.45%, Achievement 47.16%. Reproduced here as a
	// single synthetic row carrying those exact sums (the formula operates
	// on sums, not per-row averages, so one row is sufficient to verify it).
	repo := &fakeIssueSessionRepoForReport{sessions: []issue_session_dto.IssueSession{
		{AssigneeEmail: "dev@esb.co.id", IssueKey: "AGG-1", SprintName: "Sprint 15-20", StoryPoints: 401.30, AccumulatedSeconds: int64(503.42 * 3600)},
	}}
	svc := newTestReportService(repo)

	report, err := svc.GenerateReport(context.Background(), "dev@esb.co.id", []string{"Sprint 15-20"}, 12)
	if err != nil {
		t.Fatalf("GenerateReport returned error: %v", err)
	}
	if !almostEqual(report.AggregateDeviationPercent, 25.45) {
		t.Errorf("AggregateDeviationPercent = %.4f, want ~25.45", report.AggregateDeviationPercent)
	}
	if !almostEqual(report.AchievementPercent, 47.15) {
		// PDF displays 47.16 (rounded from its own already-rounded 25.45%
		// deviation); computed from full precision here, the two agree to
		// within normal rounding tolerance.
		t.Errorf("AchievementPercent = %.4f, want ~47.15", report.AchievementPercent)
	}
}

// A deviation under the cap must not blow past 100% (cap 10%, deviation
// 0.086% used to show "11,617%").
func TestAchievementPercent_CappedAt100(t *testing.T) {
	cases := map[float64]float64{0: 100, 0.086: 100, 10: 100, 20: 50}
	for deviation, want := range cases {
		if got := achievementPercent(10, deviation); !almostEqual(got, want) {
			t.Errorf("achievementPercent(10, %v) = %v, want %v", deviation, got, want)
		}
	}
}

func TestGenerateReport_WithinBandCounting(t *testing.T) {
	repo := &fakeIssueSessionRepoForReport{sessions: []issue_session_dto.IssueSession{
		{AssigneeEmail: "dev@esb.co.id", IssueKey: "A", SprintName: "S1", StoryPoints: 10, AccumulatedSeconds: 10 * 3600}, // 0% deviation -> WITHIN
		{AssigneeEmail: "dev@esb.co.id", IssueKey: "B", SprintName: "S1", StoryPoints: 10, AccumulatedSeconds: 20 * 3600}, // 100% deviation -> OUT
	}}
	svc := newTestReportService(repo)

	report, err := svc.GenerateReport(context.Background(), "dev@esb.co.id", []string{"S1"}, 12)
	if err != nil {
		t.Fatalf("GenerateReport returned error: %v", err)
	}
	if report.WithinCount != 1 || report.TotalCount != 2 {
		t.Errorf("WithinCount=%d TotalCount=%d, want 1, 2", report.WithinCount, report.TotalCount)
	}
}

func TestGenerateReport_UsesLiveElapsedForActiveSession(t *testing.T) {
	// AC7.5: a still-running session must use CalculateElapsedSeconds
	// (accumulated + live business-hours since start), not raw
	// accumulatedSeconds alone.
	start := time.Now() // business_hours treats "right now" sanely regardless of weekday for a 0-elapsed sanity check
	repo := &fakeIssueSessionRepoForReport{sessions: []issue_session_dto.IssueSession{
		{AssigneeEmail: "dev@esb.co.id", IssueKey: "LIVE-1", SprintName: "S1", StoryPoints: 1, AccumulatedSeconds: 1800, CurrentSessionStartedDate: &start},
	}}
	svc := newTestReportService(repo)

	report, err := svc.GenerateReport(context.Background(), "dev@esb.co.id", []string{"S1"}, 12)
	if err != nil {
		t.Fatalf("GenerateReport returned error: %v", err)
	}
	// At minimum, ActualHours must be >= the banked 1800s (0.5h) — it can
	// only grow from live elapsed time, never shrink.
	if report.Rows[0].ActualHours < 0.5 {
		t.Errorf("ActualHours = %.4f, want >= 0.5 (accumulated alone)", report.Rows[0].ActualHours)
	}
}

func TestGenerateReport_EmptyWhenNoSessionsMatch(t *testing.T) {
	repo := &fakeIssueSessionRepoForReport{}
	svc := newTestReportService(repo)

	report, err := svc.GenerateReport(context.Background(), "dev@esb.co.id", []string{"Sprint Nonexistent"}, 12)
	if err != nil {
		t.Fatalf("GenerateReport returned error: %v", err)
	}
	if len(report.Rows) != 0 || report.TotalCount != 0 {
		t.Errorf("report = %+v, want empty", report)
	}
}

func TestListAvailableSprints_OnlySprintsWhereCallerHasTickets(t *testing.T) {
	repo := &fakeIssueSessionRepoForReport{
		distinctSprintNames: []string{"Sprint 20", "Sprint 19"},
		sessions: []issue_session_dto.IssueSession{
			{AssigneeEmail: "dev@esb.co.id", IssueKey: "A", SprintName: "Sprint 20", StoryPoints: 1},
			{AssigneeEmail: "other@esb.co.id", IssueKey: "B", SprintName: "Sprint 19", StoryPoints: 1},
		},
	}
	svc := newTestReportService(repo)

	got, err := svc.ListAvailableSprints(context.Background(), "dev@esb.co.id")
	if err != nil {
		t.Fatalf("ListAvailableSprints returned error: %v", err)
	}
	if len(got) != 1 || got[0] != "Sprint 20" {
		t.Errorf("got = %v, want only [Sprint 20] (Sprint 19 only has another user's tickets)", got)
	}
}

func TestGenerateReport_ExcludesOtherUsersTickets(t *testing.T) {
	repo := &fakeIssueSessionRepoForReport{sessions: []issue_session_dto.IssueSession{
		{AssigneeEmail: "dev@esb.co.id", IssueKey: "MINE", SprintName: "S1", StoryPoints: 10, AccumulatedSeconds: 10 * 3600},
		{AssigneeEmail: "other@esb.co.id", IssueKey: "THEIRS", SprintName: "S1", StoryPoints: 10, AccumulatedSeconds: 10 * 3600},
	}}
	svc := newTestReportService(repo)

	report, err := svc.GenerateReport(context.Background(), "dev@esb.co.id", []string{"S1"}, 12)
	if err != nil {
		t.Fatalf("GenerateReport returned error: %v", err)
	}
	if report.TotalCount != 1 || len(report.Rows) != 1 || report.Rows[0].IssueKey != "MINE" {
		t.Errorf("report = %+v, want only the caller's own ticket MINE", report)
	}
}

func (f *fakeIssueSessionRepoForReport) UpdateActiveSprint(ctx context.Context, issueKey string, boardID, sprintID *int64) error {
	return nil
}
