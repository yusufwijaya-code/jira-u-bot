package user_dashboard_service

import (
	"context"
	"log"
	"strings"
	"time"

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
}

func NewUserDashboardService(
	jiraClient jira_client_service.JiraClientService,
	jiraCredential jira_credential_service.JiraCredentialService,
	registrationRepo registration_repository.RegistrationRepository,
	issueSessionRepo issue_session_repository.IssueSessionRepository,
	businessHours business_hours_service.BusinessHoursService,
	holidayCalendar business_hours_dto.HolidayCalendar,
) UserDashboardService {
	return &userDashboardServiceImpl{
		jiraClient:       jiraClient,
		jiraCredential:   jiraCredential,
		registrationRepo: registrationRepo,
		issueSessionRepo: issueSessionRepo,
		businessHours:    businessHours,
		holidayCalendar:  holidayCalendar,
	}
}

func (s *userDashboardServiceImpl) GetMyBoard(ctx context.Context, email string) (user_dashboard_dto.Board, error) {
	user, found, err := s.registrationRepo.FindUserByEmail(ctx, email)
	if err != nil {
		return user_dashboard_dto.Board{}, err
	}
	if !found {
		return user_dashboard_dto.Board{}, nil
	}

	// A credential resolution error (e.g. decrypt failure) falls back to
	// the central credential rather than failing the whole board — spec
	// AC5.4, same resilience contract as the OAuth DM fallback precedent.
	cred, credErr := s.jiraCredential.ResolveCredential(ctx, email)
	if credErr != nil {
		log.Printf("user_dashboard_service: ResolveCredential(%s) failed, falling back to central credential: %v", email, credErr)
	}

	issues, err := s.jiraClient.SearchAllIssuesByAssignee(ctx, cred, user.JiraAccountID)
	if err != nil {
		return user_dashboard_dto.Board{}, err
	}

	board := user_dashboard_dto.Board{}
	columnIndex := map[string]int{}
	for _, issue := range issues {
		card := s.buildCard(ctx, issue)

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

// buildCard implements spec AC3.2: a Code Process ticket with a tracked
// session gets budget detail attached; every other ticket (including a
// Code Process ticket the poller hasn't captured yet — a brief race, not
// an error) gets just the basics.
func (s *userDashboardServiceImpl) buildCard(ctx context.Context, issue jira_client_dto.Issue) user_dashboard_dto.Card {
	card := user_dashboard_dto.Card{
		IssueKey: issue.Key,
		Summary:  issue.Fields.Summary,
		Status:   issue.Fields.Status.Name,
	}

	if !strings.EqualFold(card.Status, jira_client_dto.StatusCodeProcess) {
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

func (s *userDashboardServiceImpl) SetReminderTierPreference(ctx context.Context, email, csv string) error {
	return s.registrationRepo.UpdateReminderTierPreference(ctx, email, csv)
}

func (s *userDashboardServiceImpl) GetReminderTierPreference(ctx context.Context, email string) (*string, error) {
	user, found, err := s.registrationRepo.FindUserByEmail(ctx, email)
	if err != nil || !found {
		return nil, err
	}
	return user.ReminderTierPreference, nil
}
