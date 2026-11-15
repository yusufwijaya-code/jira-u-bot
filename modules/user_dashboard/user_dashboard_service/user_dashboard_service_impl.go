package user_dashboard_service

import (
	"context"
	"errors"
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
	holidayCalendar  business_hours_dto.CalendarSource
	jiraBaseURL      string
	agile            activeSprintSearcher
	boards           boardDirectory
}

// activeSprintSearcher is the one jira_client_service.JiraAgileClient
// capability this service needs.
type activeSprintSearcher interface {
	SearchActiveSprintIssuesByAssignee(ctx context.Context, cred *jira_client_dto.Credential, accountID string) ([]jira_client_dto.Issue, error)
	SearchIssuesByKeysForAssignee(ctx context.Context, cred *jira_client_dto.Credential, keys []string, accountID string) ([]jira_client_dto.Issue, error)
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
	holidayCalendar business_hours_dto.CalendarSource,
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

// sprintQuery is email's active-sprint tickets plus what a follow-up Jira
// query on their behalf needs.
type sprintQuery struct {
	issues    []jira_client_dto.Issue
	cred      *jira_client_dto.Credential
	accountID string
}

// activeSprintIssues returns email's tickets in open sprints (empty for an
// unregistered user).
func (s *userDashboardServiceImpl) activeSprintIssues(ctx context.Context, email string) (sprintQuery, error) {
	user, found, err := s.registrationRepo.FindUserByEmail(ctx, email)
	if err != nil {
		return sprintQuery{}, err
	}
	if !found {
		return sprintQuery{}, nil
	}

	// A credential resolution error (e.g. decrypt failure) falls back to
	// the central credential rather than failing the whole board — spec
	// AC5.4, same resilience contract as the OAuth DM fallback precedent.
	cred, credErr := s.jiraCredential.ResolveCredential(ctx, email)
	if credErr != nil {
		log.Printf("user_dashboard_service: ResolveCredential(%s) failed, falling back to central credential: %v", email, credErr)
	}
	issues, err := s.agile.SearchActiveSprintIssuesByAssignee(ctx, cred, user.JiraAccountID)
	if err != nil {
		return sprintQuery{}, err
	}
	return sprintQuery{issues: issues, cred: cred, accountID: user.JiraAccountID}, nil
}

// branchParent is the active-sprint ticket a branch hangs under.
type branchParent struct {
	issue    jira_client_dto.Issue
	relation string
}

func (p branchParent) applyTo(card *user_dashboard_dto.Card) {
	card.ParentKey = p.issue.Key
	card.ParentSummary = p.issue.Fields.Summary
	card.ParentStatus = p.issue.Fields.Status.Name
	card.Relation = p.relation
}

// relatedCards finds the caller's subtasks/linked items (any status) of
// their active-sprint tickets that aren't in the sprint themselves (Tech
// Spec "Subtask & Linked Item Branches") — the dashboard hangs them under
// their parent in "Tiket saya per status" and, when tracked, in the
// active/review cards. Each hangs under the
// first sprint ticket that references it and takes that ticket's board. A
// Jira failure only drops the branches (AC1.8).
func (s *userDashboardServiceImpl) relatedCards(ctx context.Context, q sprintQuery, boardID int64) []user_dashboard_dto.Card {
	inSprint := make(map[string]bool, len(q.issues))
	for _, issue := range q.issues {
		inSprint[issue.Key] = true
	}
	parents := map[string]branchParent{}
	var keys []string
	for _, issue := range q.issues {
		for _, rel := range issue.Relations() {
			if inSprint[rel.Key] {
				continue
			}
			if _, seen := parents[rel.Key]; seen {
				continue
			}
			parents[rel.Key] = branchParent{issue: issue, relation: rel.Label}
			keys = append(keys, rel.Key)
		}
	}
	if len(keys) == 0 {
		return nil
	}

	issues, err := s.agile.SearchIssuesByKeysForAssignee(ctx, q.cred, keys, q.accountID)
	if err != nil {
		log.Printf("user_dashboard_service: related items lookup failed, showing no branches: %v", err)
		return nil
	}

	var cards []user_dashboard_dto.Card
	for _, issue := range issues {
		parent, ok := parents[issue.Key]
		if !ok {
			continue
		}
		if a := issue.Fields.Assignee; a != nil && a.AccountID != "" && a.AccountID != q.accountID {
			continue
		}
		sprint, inBoard := activeBoardOf(parent.issue)
		if boardID > 0 && (!inBoard || sprint.BoardID != boardID) {
			continue
		}
		card := s.buildCard(ctx, issue)
		if inBoard {
			card.BoardID = sprint.BoardID
		}
		parent.applyTo(&card)
		cards = append(cards, card)
	}
	return cards
}

func activeBoardOf(issue jira_client_dto.Issue) (jira_client_dto.SprintRef, bool) {
	return issue.Fields.ActiveSprint()
}

func (s *userDashboardServiceImpl) GetMyBoard(ctx context.Context, email string, boardID int64) (user_dashboard_dto.Board, error) {
	q, err := s.activeSprintIssues(ctx, email)
	if err != nil {
		return user_dashboard_dto.Board{}, err
	}

	byKey := make(map[string]jira_client_dto.Issue, len(q.issues))
	for _, issue := range q.issues {
		byKey[issue.Key] = issue
	}

	board := user_dashboard_dto.Board{}
	columnIndex := map[string]int{}
	for _, issue := range q.issues {
		sprint, inSprint := activeBoardOf(issue)
		if boardID > 0 && (!inSprint || sprint.BoardID != boardID) {
			continue
		}
		card := s.buildCard(ctx, issue)
		if inSprint {
			card.BoardID = sprint.BoardID
		}
		// A sprint ticket that is a subtask of another sprint ticket hangs
		// under it (AC2.4); links between sprint tickets don't nest.
		if p := issue.Fields.Parent; p != nil {
			if parent, ok := byKey[p.Key]; ok {
				branchParent{issue: parent, relation: jira_client_dto.RelationSubtask}.applyTo(&card)
			}
		}

		idx, ok := columnIndex[card.Status]
		if !ok {
			idx = len(board.Columns)
			columnIndex[card.Status] = idx
			board.Columns = append(board.Columns, user_dashboard_dto.BoardColumn{Status: card.Status})
		}
		board.Columns[idx].Cards = append(board.Columns[idx].Cards, card)
	}
	board.Related = s.relatedCards(ctx, q, boardID)
	return board, nil
}

func (s *userDashboardServiceImpl) DescribeBoard(ctx context.Context, boardID int64) (string, string) {
	name := fmt.Sprintf("Board #%d", boardID)
	if s.boards == nil {
		return name, ""
	}
	b, err := s.boards.EnsureKnown(ctx, boardID)
	if err != nil {
		log.Printf("user_dashboard_service: board %d lookup failed: %v", boardID, err)
		return name, ""
	}
	if b.Name != "" {
		name = b.Name
	}
	return name, b.ProjectKey
}

func (s *userDashboardServiceImpl) ListBoards(ctx context.Context, email string) ([]user_dashboard_dto.BoardSummary, error) {
	q, err := s.activeSprintIssues(ctx, email)
	if err != nil {
		return nil, err
	}
	issues := q.issues

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
	if sp := issue.Fields.EffectiveStoryPoints(); sp != nil {
		card.StoryPoints = *sp
	}

	session, found, err := s.issueSessionRepo.GetSessionByIssueKey(ctx, issue.Key)
	if err != nil || !found {
		return card
	}

	// Budget detail for every status whose time counts — CORE also counts
	// CODE REVIEW (Tech Spec "CORE: Waktu Code Review Ikut Dihitung").
	if jira_client_dto.RuleForIssue(issue.Key).IsTracked(card.Status) && session.FlagActive {
		s.attachBudgetDetail(ctx, &card, *session)
		return card
	}

	// Clock stopped (finished / back to To Do): show what was recorded.
	if session.AccumulatedSeconds > 0 && session.StoryPoints > 0 {
		card.TimeRecorded = true
		card.StoryPoints = session.StoryPoints
		card.ElapsedSeconds = session.AccumulatedSeconds
		card.BudgetSeconds = int64(session.StoryPoints * 3600)
		card.ElapsedLabel = google_chat_notifier_service.FormatDuration(card.ElapsedSeconds)
		card.RemainingLabel = google_chat_notifier_service.FormatDuration(card.BudgetSeconds - card.ElapsedSeconds)
	}
	return card
}

func (s *userDashboardServiceImpl) attachBudgetDetail(ctx context.Context, card *user_dashboard_dto.Card, session issue_session_dto.IssueSession) {
	cal := s.holidayCalendar.ForEmail(ctx, session.AssigneeEmail)
	budgetSeconds := int64(session.StoryPoints * 3600)
	elapsedSeconds := issue_session_service.CalculateElapsedSeconds(session, s.businessHours, cal)
	remainingSeconds := budgetSeconds - elapsedSeconds
	deadline := s.businessHours.DeadlineAfterBusinessSeconds(time.Now(), remainingSeconds, cal)

	card.HasBudgetDetail = true
	card.StoryPoints = session.StoryPoints
	card.ElapsedLabel = google_chat_notifier_service.FormatDuration(elapsedSeconds)
	card.RemainingLabel = google_chat_notifier_service.FormatDuration(remainingSeconds)
	card.DeadlineLabel = google_chat_notifier_service.FormatDeadline(deadline)
	card.ElapsedSeconds = elapsedSeconds
	card.BudgetSeconds = budgetSeconds
	card.DeadlineAt = deadline.Format(time.RFC3339)
	if session.CurrentSessionStartedDate != nil && !session.CurrentSessionStartedDate.IsZero() {
		card.StartedAt = session.CurrentSessionStartedDate.Format(time.RFC3339)
	}
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
		group.Cards = append(group.Cards, s.cardFromSession(ctx, session))
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
		elapsedSeconds := issue_session_service.CalculateElapsedSeconds(session, s.businessHours, s.holidayCalendar.ForEmail(ctx, session.AssigneeEmail))
		if elapsedSeconds <= budgetSeconds {
			continue
		}
		overdue = append(overdue, user_dashboard_dto.OverdueItem{
			Card:          s.cardFromSession(ctx, session),
			AssigneeEmail: session.AssigneeEmail,
		})
	}
	return overdue, nil
}

// cardFromSession builds a Card straight from a tracked session (Team and
// Overdue views only ever show tickets that already have one, unlike
// buildCard for My Board which starts from a live Jira issue).
func (s *userDashboardServiceImpl) cardFromSession(ctx context.Context, session issue_session_dto.IssueSession) user_dashboard_dto.Card {
	card := user_dashboard_dto.Card{
		IssueKey: session.IssueKey,
		Summary:  session.IssueSummary,
		Status:   jira_client_dto.StatusCodeProcess,
	}
	s.attachBudgetDetail(ctx, &card, session)
	return card
}

var (
	// ErrNotOwnTicket: the ticket isn't one of the caller's active-sprint
	// tickets (Tech Spec "Papan Tiket & Cuti", AC1.6).
	ErrNotOwnTicket = errors.New("user_dashboard: not the caller's active-sprint ticket")
	// ErrTransitionUnavailable: the transition isn't offered by the
	// ticket's workflow from its current status.
	ErrTransitionUnavailable = errors.New("user_dashboard: transition not available from the current status")
)

// ownSprintTicket finds issueKey among email's active-sprint tickets
// (subtasks inherit their parent's sprint, so they're included).
func (s *userDashboardServiceImpl) ownSprintTicket(ctx context.Context, email, issueKey string) (jira_client_dto.Issue, *jira_client_dto.Credential, error) {
	q, err := s.activeSprintIssues(ctx, email)
	if err != nil {
		return jira_client_dto.Issue{}, nil, err
	}
	for _, issue := range q.issues {
		if strings.EqualFold(issue.Key, issueKey) {
			return issue, q.cred, nil
		}
	}
	return jira_client_dto.Issue{}, nil, ErrNotOwnTicket
}

func (s *userDashboardServiceImpl) ListTransitions(ctx context.Context, email, issueKey string) (user_dashboard_dto.TicketTransitions, error) {
	issue, cred, err := s.ownSprintTicket(ctx, email, issueKey)
	if err != nil {
		return user_dashboard_dto.TicketTransitions{}, err
	}
	transitions, err := s.jiraClient.GetTransitionsAs(ctx, cred, issue.Key)
	if err != nil {
		return user_dashboard_dto.TicketTransitions{}, err
	}
	out := user_dashboard_dto.TicketTransitions{
		IssueKey:      issue.Key,
		CurrentStatus: issue.Fields.Status.Name,
		Transitions:   make([]user_dashboard_dto.TransitionOption, 0, len(transitions)),
	}
	for _, tr := range transitions {
		out.Transitions = append(out.Transitions, user_dashboard_dto.TransitionOption{ID: tr.ID, Name: tr.Name, ToStatus: tr.To.Name})
	}
	return out, nil
}

func (s *userDashboardServiceImpl) Transition(ctx context.Context, email, issueKey, transitionID string) (string, error) {
	issue, cred, err := s.ownSprintTicket(ctx, email, issueKey)
	if err != nil {
		return "", err
	}
	// Re-read the workflow: the ticket may have moved since the menu opened.
	transitions, err := s.jiraClient.GetTransitionsAs(ctx, cred, issue.Key)
	if err != nil {
		return "", err
	}
	for _, tr := range transitions {
		if tr.ID == transitionID {
			if err := s.jiraClient.TransitionIssueAs(ctx, cred, issue.Key, tr.ID); err != nil {
				return "", err
			}
			return tr.To.Name, nil
		}
	}
	return "", ErrTransitionUnavailable
}
