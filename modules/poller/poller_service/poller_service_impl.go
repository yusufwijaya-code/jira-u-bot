package poller_service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"nudge/modules/board/board_dto"
	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/issue_session/issue_session_dto"
	"nudge/modules/issue_session/issue_session_service"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_client_service"
	"nudge/modules/registration/registration_repository"
	"nudge/modules/reminder_engine/reminder_engine_service"
)

const pollInterval = 10 * time.Second

type pollerServiceImpl struct {
	jiraClient       jira_client_service.JiraClientService
	issueSession     issue_session_service.IssueSessionService
	businessHours    business_hours_service.BusinessHoursService
	reminderEngine   reminder_engine_service.ReminderEngineService
	registrationRepo registration_repository.RegistrationRepository
	holidayCalendar  business_hours_dto.HolidayCalendar
	// boards caches the Jira Agile board of each ticket's active sprint;
	// optional (nil = board names are never fetched).
	boards boardRegistry
}

// boardRegistry is the one board_service capability the poller needs.
type boardRegistry interface {
	EnsureKnown(ctx context.Context, boardID int64) (board_dto.Board, error)
}

func NewPollerService(
	jiraClient jira_client_service.JiraClientService,
	issueSession issue_session_service.IssueSessionService,
	businessHours business_hours_service.BusinessHoursService,
	reminderEngine reminder_engine_service.ReminderEngineService,
	registrationRepo registration_repository.RegistrationRepository,
	holidayCalendar business_hours_dto.HolidayCalendar,
	boards boardRegistry,
) PollerService {
	return &pollerServiceImpl{
		jiraClient:       jiraClient,
		issueSession:     issueSession,
		businessHours:    businessHours,
		reminderEngine:   reminderEngine,
		registrationRepo: registrationRepo,
		holidayCalendar:  holidayCalendar,
		boards:           boards,
	}
}

// resolveEligibility implements the team's decision to scope the bot down
// from "every CORE/OMS ticket in Code Process" to only tickets assigned to
// a registered user with a real Story Points budget — an unassigned
// ticket, an assignee who never used /register, or a ticket with no (or
// zero) SP carries nothing meaningful to track and must never reach the
// session/reminder pipeline at all (not even to show up as an instant
// overage).
//
// Matching is done by the assignee's Jira accountId, not their email —
// this Jira org hides assignee.emailAddress for any account but the API
// token's own (confirmed live), so accountId is the only assignee
// identifier that's ever reliably populated. On a match, the REGISTERED
// email (typed at registration, already verified against Jira) is
// returned for session tracking/DM routing — never the issue's own
// (often blank) emailAddress field.
func (p *pollerServiceImpl) resolveEligibility(ctx context.Context, issue jira_client_dto.Issue) (assigneeEmail string, eligible bool) {
	sp := issue.Fields.EffectiveStoryPoints()
	if sp == nil || *sp <= 0 {
		return "", false
	}
	if issue.Fields.Assignee == nil {
		return "", false
	}
	user, found, err := p.registrationRepo.FindUserByJiraAccountID(ctx, issue.Fields.Assignee.AccountID)
	if err != nil || !found {
		return "", false
	}
	// Tech Spec "Dashboard Jira Manage", Chapter 10a bug fix: a deactivated
	// user (admin toggle, AC6.2) must stop being tracked/reminded, not just
	// lose login access — FindUserByJiraAccountID doesn't filter on
	// flagActive itself, so this has to be checked here.
	if !user.FlagActive {
		return "", false
	}
	return user.Email, true
}

// filterEligible returns the eligible subset of issues alongside a
// issueKey -> registered-email map (see resolveEligibility) for callers
// that need the resolved email when opening/updating sessions.
func (p *pollerServiceImpl) filterEligible(ctx context.Context, issues []jira_client_dto.Issue) ([]jira_client_dto.Issue, map[string]string) {
	eligible := make([]jira_client_dto.Issue, 0, len(issues))
	resolvedEmails := make(map[string]string, len(issues))
	for _, issue := range issues {
		email, ok := p.resolveEligibility(ctx, issue)
		if !ok {
			continue
		}
		eligible = append(eligible, issue)
		resolvedEmails[issue.Key] = email
	}
	return eligible, resolvedEmails
}

func (p *pollerServiceImpl) Run(ctx context.Context) {
	if err := p.backfillActiveSessions(ctx); err != nil {
		log.Printf("poller: startup backfill failed: %v", err)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.pollOnce(ctx); err != nil {
				log.Printf("poller: cycle failed: %v", err)
			}
		}
	}
}

func (p *pollerServiceImpl) pollOnce(ctx context.Context) error {
	issues, err := p.jiraClient.SearchCodeProcessIssues(ctx)
	if err != nil {
		return fmt.Errorf("poller: SearchCodeProcessIssues failed: %w", err)
	}
	issues, resolvedEmails := p.filterEligible(ctx, issues)

	currentByKey := make(map[string]jira_client_dto.Issue, len(issues))
	for _, issue := range issues {
		currentByKey[issue.Key] = issue
	}

	activeSessions, err := p.issueSession.GetActiveSessions(ctx)
	if err != nil {
		return fmt.Errorf("poller: GetActiveSessions failed: %w", err)
	}
	activeByKey := make(map[string]issue_session_dto.IssueSession, len(activeSessions))
	for _, s := range activeSessions {
		activeByKey[s.IssueKey] = s
	}

	// Exit events: DB still has it active, Jira no longer reports it in one
	// of its project's tracked statuses this cycle (OMS: Code Process; CORE:
	// Code Process or CODE REVIEW — moving between those keeps it here).
	for key, session := range activeByKey {
		if _, stillThere := currentByKey[key]; stillThere {
			continue
		}
		additionalSeconds := int64(0)
		if session.CurrentSessionStartedDate != nil {
			additionalSeconds = p.businessHours.ElapsedBusinessSeconds(*session.CurrentSessionStartedDate, time.Now(), p.holidayCalendar)
		}
		totalElapsed := session.AccumulatedSeconds + additionalSeconds
		budgetSeconds := int64(session.StoryPoints * 3600)

		if err := p.issueSession.CloseSession(ctx, key, additionalSeconds); err != nil {
			return fmt.Errorf("poller: CloseSession(%s) failed: %w", key, err)
		}

		// Exit-destination notices (spec AC4.1/AC4.4/AC4.5, plus the To Do
		// case for /u-geser and manual Jira changes) are best-effort: the
		// session above is already closed by the time we get here, so there
		// is no natural retry hook if either step below fails — log and move
		// on to the next ticket rather than failing the cycle.
		status, err := p.jiraClient.GetIssueStatus(ctx, key)
		if err != nil {
			log.Printf("poller: GetIssueStatus(%s) failed, skipping exit-destination notice: %v", key, err)
			continue
		}
		var startedAt time.Time
		if session.CurrentSessionStartedDate != nil {
			startedAt = *session.CurrentSessionStartedDate
		}
		// Tech Spec "CORE: Waktu Code Review Ikut Dihitung" AC1.4: the
		// project's "done" status (OMS: Code Review, CORE: Code Completed)
		// means finished; To Do means pulled back; anything else just stops
		// the clock silently. Matched case-insensitively.
		rule := jira_client_dto.RuleForIssue(key)
		switch {
		case rule.IsDone(status):
			// finishedAt is "now": GetIssueStatus above just confirmed the
			// ticket left its tracked statuses this cycle — the closest we
			// track to the actual transition.
			if err := p.reminderEngine.NotifyIfEarlyCompletion(ctx, key, session.AssigneeEmail, session.IssueSummary, totalElapsed, budgetSeconds, startedAt, time.Now()); err != nil {
				log.Printf("poller: NotifyIfEarlyCompletion(%s) failed: %v", key, err)
			}
		case strings.EqualFold(status, jira_client_dto.StatusToDo):
			if err := p.reminderEngine.NotifyReturnedToTodo(ctx, key, session.AssigneeEmail, session.IssueSummary, totalElapsed, budgetSeconds, startedAt); err != nil {
				log.Printf("poller: NotifyReturnedToTodo(%s) failed: %v", key, err)
			}
		}
	}

	// Entry events: new to the DB this cycle — open a fresh session at the
	// REAL Code Process entry time, not now. A ticket may have already been
	// in Code Process for a while before this is its first eligible cycle
	// (e.g. the assignee only just registered) — using CURRENT_TIMESTAMP
	// here would silently reset elapsed-time tracking to zero.
	for key, issue := range currentByKey {
		if _, alreadyActive := activeByKey[key]; alreadyActive {
			continue
		}
		entryTime, err := p.sessionStartTime(ctx, key)
		if err != nil {
			return err
		}
		if err := p.issueSession.OpenOrResumeSessionAt(ctx, key, projectKeyFromIssueKey(key), storyPointsOf(issue), entryTime, resolvedEmails[key], issue.Fields.Summary, sprintNameOf(issue)); err != nil {
			return fmt.Errorf("poller: OpenOrResumeSessionAt(%s) failed: %w", key, err)
		}
	}

	// Steady-state: was already active — refresh SP/assignee/summary (spec
	// Note C3, and spec Business Logic for Early Completion Notice), NEVER
	// call OpenOrResumeSession here (it would reset currentSessionStartedDate).
	for key, issue := range currentByKey {
		if _, wasAlreadyActive := activeByKey[key]; !wasAlreadyActive {
			continue
		}
		if err := p.issueSession.UpdateStoryPoints(ctx, key, storyPointsOf(issue), resolvedEmails[key], issue.Fields.Summary, sprintNameOf(issue)); err != nil {
			return fmt.Errorf("poller: UpdateStoryPoints(%s) failed: %w", key, err)
		}
	}

	// Evaluate reminders for every ticket currently in Code Process.
	for key, issue := range currentByKey {
		session, found, err := p.issueSession.GetSessionByIssueKey(ctx, key)
		if err != nil {
			return fmt.Errorf("poller: GetSessionByIssueKey(%s) failed: %w", key, err)
		}
		if !found {
			continue
		}
		if err := p.recordActiveSprint(ctx, issue, session); err != nil {
			return fmt.Errorf("poller: UpdateActiveSprint(%s) failed: %w", key, err)
		}
		if err := p.reminderEngine.EvaluateAndSend(ctx, issue, *session); err != nil {
			return fmt.Errorf("poller: EvaluateAndSend(%s) failed: %w", key, err)
		}
	}

	return nil
}

// backfillActiveSessions runs once at startup (spec AC1.4): any ticket Jira
// already reports as "Code Process" that this process has no DB row for yet
// (fresh deploy, or DB was reset) gets its session opened at the REAL
// changelog transition time, not at process-start time — otherwise a ticket
// that has been in Code Process for days would appear to have just started.
func (p *pollerServiceImpl) backfillActiveSessions(ctx context.Context) error {
	issues, err := p.jiraClient.SearchCodeProcessIssues(ctx)
	if err != nil {
		return fmt.Errorf("poller: backfill SearchCodeProcessIssues failed: %w", err)
	}
	issues, resolvedEmails := p.filterEligible(ctx, issues)

	for _, issue := range issues {
		_, found, err := p.issueSession.GetSessionByIssueKey(ctx, issue.Key)
		if err != nil {
			return fmt.Errorf("poller: backfill GetSessionByIssueKey(%s) failed: %w", issue.Key, err)
		}
		if found {
			continue
		}

		entryTime, err := p.sessionStartTime(ctx, issue.Key)
		if err != nil {
			return fmt.Errorf("poller: backfill %w", err)
		}

		if err := p.issueSession.OpenOrResumeSessionAt(ctx, issue.Key, projectKeyFromIssueKey(issue.Key), storyPointsOf(issue), entryTime, resolvedEmails[issue.Key], issue.Fields.Summary, sprintNameOf(issue)); err != nil {
			return fmt.Errorf("poller: backfill OpenOrResumeSessionAt(%s) failed: %w", issue.Key, err)
		}
	}

	return nil
}

// recordActiveSprint keeps tr_issuesession.boardID/sprintID in sync with the
// ticket's ACTIVE sprint (Tech Spec "Per-Board Settings & Active Sprint
// Dashboard", AC5.1) and updates session in place so the reminder engine
// immediately uses that board's settings. It only writes when something
// changed, so steady state costs no extra queries. An unknown board is
// cached via boards.EnsureKnown; a failure there is logged, never fatal.
func (p *pollerServiceImpl) recordActiveSprint(ctx context.Context, issue jira_client_dto.Issue, session *issue_session_dto.IssueSession) error {
	var boardID, sprintID *int64
	if sprint, ok := issue.Fields.ActiveSprint(); ok {
		b, sp := sprint.BoardID, sprint.ID
		boardID, sprintID = &b, &sp
	}
	if sameID(session.BoardID, boardID) && sameID(session.SprintID, sprintID) {
		return nil
	}
	if boardID != nil && p.boards != nil {
		if _, err := p.boards.EnsureKnown(ctx, *boardID); err != nil {
			log.Printf("poller: board %d lookup failed (retrying next cycle): %v", *boardID, err)
		}
	}
	if err := p.issueSession.UpdateActiveSprint(ctx, issue.Key, boardID, sprintID); err != nil {
		return err
	}
	session.BoardID, session.SprintID = boardID, sprintID
	return nil
}

func sameID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// sessionStartTime picks when a (re)opened session's clock starts (Tech Spec
// "CORE: Waktu Code Review Ikut Dihitung", AC1.5):
//   - a ticket never tracked before starts at the beginning of its current
//     run of tracked statuses — for CORE that includes Code Process time
//     before it moved to CODE REVIEW;
//   - a ticket whose (closed) session is being reopened starts at its latest
//     entry into a tracked status, so time already accumulated is never
//     counted twice (e.g. a CORE ticket closed when it entered review under
//     the old rules, reopened by this version).
//
// No changelog entry → now.
func (p *pollerServiceImpl) sessionStartTime(ctx context.Context, issueKey string) (time.Time, error) {
	runStart, lastEntry, found, err := p.jiraClient.GetTrackedRunTimes(ctx, issueKey)
	if err != nil {
		return time.Time{}, fmt.Errorf("poller: GetTrackedRunTimes(%s) failed: %w", issueKey, err)
	}
	if !found {
		return time.Now(), nil
	}
	_, existed, err := p.issueSession.GetSessionByIssueKey(ctx, issueKey)
	if err != nil {
		return time.Time{}, fmt.Errorf("poller: GetSessionByIssueKey(%s) failed: %w", issueKey, err)
	}
	if existed {
		return lastEntry, nil
	}
	return runStart, nil
}

func storyPointsOf(issue jira_client_dto.Issue) float64 {
	sp := issue.Fields.EffectiveStoryPoints()
	if sp == nil {
		return 0
	}
	return *sp
}

// sprintNameOf returns "" when the ticket has no sprint info (never
// assigned, or Jira didn't return it) — tr_issuesession.sprintName is
// nullable and this empty string maps to that (spec Chapter 11a).
func sprintNameOf(issue jira_client_dto.Issue) string {
	name, found := issue.Fields.ActiveSprintName()
	if !found {
		return ""
	}
	return name
}

func projectKeyFromIssueKey(issueKey string) string {
	if idx := strings.Index(issueKey, "-"); idx > 0 {
		return issueKey[:idx]
	}
	return issueKey
}
