package user_dashboard_service

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"nudge/modules/board/board_dto"
	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/google_chat_notifier/google_chat_notifier_service"
	"nudge/modules/issue_session/issue_session_dto"
	"nudge/modules/issue_session/issue_session_repository"
	"nudge/modules/issue_session/issue_session_service"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_client_service"
	"nudge/modules/jira_credential/jira_credential_service"
	"nudge/modules/registration/registration_repository"
	"nudge/modules/user_dashboard/user_dashboard_dto"
)

type userDashboardServiceImpl struct {
	jiraClient       jira_client_service.JiraClientService
	jiraCredential   jira_credential_service.JiraCredentialService
	registrationRepo registration_repository.RegistrationRepository
	issueSessionRepo issue_session_repository.IssueSessionRepository
	businessHours    business_hours_service.BusinessHoursService
	holidayCalendar  business_hours_dto.HolidayCalendar
	jiraBaseURL      string
	agile            activeSprintSearcher
	boards           boardDirectory
}

// activeSprintSearcher is the one jira_client_service.JiraAgileClient
// capability this service needs.
type activeSprintSearcher interface {
	SearchActiveSprintIssuesByAssignee(ctx context.Context, cred *jira_client_dto.Credential, accountID string) ([]jira_client_dto.Issue, error)
}

// boardDirectory is the board_service slice this service needs.
type boardDirectory interface {
	EnsureKnown(ctx context.Context, boardID int64) (board_dto.Board, error)
	ActiveSprint(ctx context.Context, boardID int64) (jira_client_dto.SprintRef, bool, error)
}

func NewUserDashboardService(
	jiraClient jira_client_service.JiraClientService,
	jiraCredential jira_credential_service.JiraCredentialService,
	registrationRepo registration_repository.RegistrationRepository,
	issueSessionRepo issue_session_repository.IssueSessionRepository,
	businessHours business_hours_service.BusinessHoursService,
	holidayCalendar business_hours_dto.HolidayCalendar,
	jiraBaseURL string,
	agile activeSprintSearcher,
	boards boardDirectory,
) UserDashboardService {
	return &userDashboardServiceImpl{
		jiraClient:       jiraClient,
		jiraCredential:   jiraCredential,
		registrationRepo: registrationRepo,
		issueSessionRepo: issueSessionRepo,
		businessHours:    businessHours,
		holidayCalendar:  holidayCalendar,
		jiraBaseURL:      strings.TrimRight(jiraBaseURL, "/"),
		agile:            agile,
		boards:           boards,
	}
}

func (s *userDashboardServiceImpl) issueURL(issueKey string) string {
	if s.jiraBaseURL == "" {
		return ""
	}
	return s.jiraBaseURL + "/browse/" + issueKey
}

// activeSprintIssues returns email's tickets in open sprints (empty for an
// unregistered user).
func (s *userDashboardServiceImpl) activeSprintIssues(ctx context.Context, email string) ([]jira_client_dto.Issue, error) {
	user, found, err := s.registrationRepo.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	// A credential resolution error (e.g. decrypt failure) falls back to
	// the central credential rather than failing the whole board — spec
	// AC5.4, same resilience contract as the OAuth DM fallback precedent.
	cred, credErr := s.jiraCredential.ResolveCredential(ctx, email)
	if credErr != nil {
		log.Printf("user_dashboard_service: ResolveCredential(%s) failed, falling back to central credential: %v", email, credErr)
	}
	return s.agile.SearchActiveSprintIssuesByAssignee(ctx, cred, user.JiraAccountID)
}

func activeBoardOf(issue jira_client_dto.Issue) (jira_client_dto.SprintRef, bool) {
	return issue.Fields.ActiveSprint()
}

func (s *userDashboardServiceImpl) GetMyBoard(ctx context.Context, email string, boardID int64) (user_dashboard_dto.Board, error) {
	issues, err := s.activeSprintIssues(ctx, email)
	if err != nil {
		return user_dashboard_dto.Board{}, err
	}

	board := user_dashboard_dto.Board{}
	columnIndex := map[string]int{}
	for _, issue := range issues {
		sprint, inSprint := activeBoardOf(issue)
		if boardID > 0 && (!inSprint || sprint.BoardID != boardID) {
			continue
		}
		card := s.buildCard(ctx, issue)
		if inSprint {
			card.BoardID = sprint.BoardID
		}

		idx, ok := columnIndex[card.Status]
		if !ok {
			idx = len(board.Columns)
			columnIndex[card.Status] = idx
			board.Columns = append(board.Columns, user_dashboard_dto.BoardColumn{Status: card.Status})
		}
		board.Columns[idx].Cards = append(board.Columns[idx].Cards, card)
	}
	return board, nil
}

func (s *userDashboardServiceImpl) ListBoards(ctx context.Context, email string) ([]user_dashboard_dto.BoardSummary, error) {
	issues, err := s.activeSprintIssues(ctx, email)
	if err != nil {
		return nil, err
	}

	byBoard := map[int64]*user_dashboard_dto.BoardSummary{}
	var order []int64
	for _, issue := range issues {
		sprint, ok := activeBoardOf(issue)
		if !ok {
			continue
		}
		summary, seen := byBoard[sprint.BoardID]
		if !seen {
			summary = &user_dashboard_dto.BoardSummary{
				BoardID:    sprint.BoardID,
				ProjectKey: jira_client_dto.ProjectKeyOf(issue.Key),
				ActiveSprint: user_dashboard_dto.SprintSummary{
					ID: sprint.ID, Name: sprint.Name, StartDate: sprint.StartDate, EndDate: sprint.EndDate,
				},
			}
			byBoard[sprint.BoardID] = summary
			order = append(order, sprint.BoardID)
		}
		summary.TicketCount++
	}

	out := make([]user_dashboard_dto.BoardSummary, 0, len(order))
	for _, id := range order {
		summary := *byBoard[id]
		summary.Name = fmt.Sprintf("Board #%d", id)
		if b, err := s.boards.EnsureKnown(ctx, id); err != nil {
			log.Printf("user_dashboard_service: board %d lookup failed: %v", id, err)
		} else {
			summary.Name = b.Name
			if b.ProjectKey != "" {
				summary.ProjectKey = b.ProjectKey
			}
		}
		summary.ReviewTracked = jira_client_dto.RuleFor(summary.ProjectKey).TracksReview()
		// The issue's Sprint field usually carries the dates; fall back to
		// the Agile API (cached) when it doesn't.
		if summary.ActiveSprint.StartDate == "" || summary.ActiveSprint.EndDate == "" {
			if sprint, found, err := s.boards.ActiveSprint(ctx, id); err == nil && found && sprint.ID == summary.ActiveSprint.ID {
				summary.ActiveSprint.StartDate, summary.ActiveSprint.EndDate = sprint.StartDate, sprint.EndDate
			}
		}
		out = append(out, summary)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// buildCard implements spec AC3.2: a Code Process ticket with a tracked
// session gets budget detail attached; every other ticket (including a
// Code Process ticket the poller hasn't captured yet — a brief race, not
// an error) gets just the basics.
func (s *userDashboardServiceImpl) buildCard(ctx context.Context, issue jira_client_dto.Issue) user_dashboard_dto.Card {
	card := user_dashboard_dto.Card{
		IssueKey: issue.Key,
		Summary:  issue.Fields.Summary,
		Status:   issue.Fields.Status.Name,
		IssueURL: s.issueURL(issue.Key),
	}

	// Budget detail for every status whose time counts — CORE also counts
	// CODE REVIEW (Tech Spec "CORE: Waktu Code Review Ikut Dihitung").
	if !jira_client_dto.RuleForIssue(issue.Key).IsTracked(card.Status) {
		return card
	}

	session, found, err := s.issueSessionRepo.GetSessionByIssueKey(ctx, issue.Key)
	if err != nil || !found || !session.FlagActive {
		return card
	}

	s.attachBudgetDetail(&card, *session)
	return card
}

func (s *userDashboardServiceImpl) attachBudgetDetail(card *user_dashboard_dto.Card, session issue_session_dto.IssueSession) {
	budgetSeconds := int64(session.StoryPoints * 3600)
	elapsedSeconds := issue_session_service.CalculateElapsedSeconds(session, s.businessHours, s.holidayCalendar)
	remainingSeconds := budgetSeconds - elapsedSeconds
	deadline := s.businessHours.DeadlineAfterBusinessSeconds(time.Now(), remainingSeconds, s.holidayCalendar)

	card.HasBudgetDetail = true
	card.StoryPoints = session.StoryPoints
	card.ElapsedLabel = google_chat_notifier_service.FormatDuration(elapsedSeconds)
	card.RemainingLabel = google_chat_notifier_service.FormatDuration(remainingSeconds)
	card.DeadlineLabel = google_chat_notifier_service.FormatDeadline(deadline)
	card.ElapsedSeconds = elapsedSeconds
	card.BudgetSeconds = budgetSeconds
	if card.IssueURL == "" {
		card.IssueURL = s.issueURL(session.IssueKey)
	}
}

// GetTeamView returns email's own active (Code Process) tickets, grouped by
// assignee like before — which now means a single group (the caller).
// Other users' tickets are never returned, admin included.
func (s *userDashboardServiceImpl) GetTeamView(ctx context.Context, email string) (user_dashboard_dto.TeamView, error) {
	sessions, err := s.issueSessionRepo.FindActiveByAssigneeEmail(ctx, email)
	if err != nil {
		return user_dashboard_dto.TeamView{}, err
	}

	view := user_dashboard_dto.TeamView{}
	if len(sessions) == 0 {
		return view, nil
	}
	group := user_dashboard_dto.TeamGroup{AssigneeEmail: email}
	for _, session := range sessions {
		group.Cards = append(group.Cards, s.cardFromSession(session))
	}
	view.Groups = append(view.Groups, group)
	return view, nil
}

// GetOverdueView returns only the given email's own active tickets that have
// passed their budget - never anyone else's, admin included.
func (s *userDashboardServiceImpl) GetOverdueView(ctx context.Context, email string) ([]user_dashboard_dto.OverdueItem, error) {
	sessions, err := s.issueSessionRepo.FindActiveByAssigneeEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	var overdue []user_dashboard_dto.OverdueItem
	for _, session := range sessions {
		budgetSeconds := int64(session.StoryPoints * 3600)
		elapsedSeconds := issue_session_service.CalculateElapsedSeconds(session, s.businessHours, s.holidayCalendar)
		if elapsedSeconds <= budgetSeconds {
			continue
		}
		overdue = append(overdue, user_dashboard_dto.OverdueItem{
			Card:          s.cardFromSession(session),
			AssigneeEmail: session.AssigneeEmail,
		})
	}
	return overdue, nil
}

// cardFromSession builds a Card straight from a tracked session (Team and
// Overdue views only ever show tickets that already have one, unlike
// buildCard for My Board which starts from a live Jira issue).
func (s *userDashboardServiceImpl) cardFromSession(session issue_session_dto.IssueSession) user_dashboard_dto.Card {
	card := user_dashboard_dto.Card{
		IssueKey: session.IssueKey,
		Summary:  session.IssueSummary,
		Status:   jira_client_dto.StatusCodeProcess,
	}
	s.attachBudgetDetail(&card, session)
	return card
}
