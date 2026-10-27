package chat_interactive_service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"jira-u-bot/modules/business_hours/business_hours_dto"
	"jira-u-bot/modules/business_hours/business_hours_service"
	"jira-u-bot/modules/chat_interactive/chat_interactive_dto"
	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_service"
	"jira-u-bot/modules/issue_session/issue_session_dto"
	"jira-u-bot/modules/issue_session/issue_session_repository"
	"jira-u-bot/modules/issue_session/issue_session_service"
	"jira-u-bot/modules/registration/registration_repository"
)

const registerURL = "https://labcup.tech/jira-notif/register"

type chatInteractiveServiceImpl struct {
	registrationRepo registration_repository.RegistrationRepository
	issueSessionRepo issue_session_repository.IssueSessionRepository
	businessHours    business_hours_service.BusinessHoursService
	holidayCalendar  business_hours_dto.HolidayCalendar
}

func NewChatInteractiveService(
	registrationRepo registration_repository.RegistrationRepository,
	issueSessionRepo issue_session_repository.IssueSessionRepository,
	businessHours business_hours_service.BusinessHoursService,
	holidayCalendar business_hours_dto.HolidayCalendar,
) ChatInteractiveService {
	return &chatInteractiveServiceImpl{
		registrationRepo: registrationRepo,
		issueSessionRepo: issueSessionRepo,
		businessHours:    businessHours,
		holidayCalendar:  holidayCalendar,
	}
}

func (s *chatInteractiveServiceImpl) RouteCommand(ctx context.Context, event chat_interactive_dto.ChatEvent) string {
	_, found, err := s.registrationRepo.FindUserByEmail(ctx, event.User.Email)
	if err != nil || !found {
		return fmt.Sprintf("Kamu belum terdaftar. Daftar dulu di %s ya 🙏", registerURL)
	}

	if event.AppCommandPayload == nil {
		return s.handleHelp()
	}

	switch event.AppCommandPayload.AppCommandMetadata.AppCommandID {
	case "mytickets":
		return s.handleMyTickets(ctx, event.User.Email)
	case "budget":
		return s.handleBudget(ctx, event.AppCommandPayload.ArgumentText)
	case "overdue":
		return s.handleOverdue(ctx)
	case "team":
		return s.handleTeam(ctx)
	default:
		return s.handleHelp()
	}
}

func (s *chatInteractiveServiceImpl) handleHelp() string {
	return "🤖 Command yang tersedia:\n\n" +
		"*/mytickets* — tiket Code Process kamu\n" +
		"*/budget <issueKey>* — detail budget satu tiket\n" +
		"*/overdue* — semua tiket yang lewat budget\n" +
		"*/team* — ringkasan semua tiket aktif per orang"
}

func (s *chatInteractiveServiceImpl) handleMyTickets(ctx context.Context, email string) string {
	sessions, err := s.issueSessionRepo.FindActiveByAssigneeEmail(ctx, email)
	if err != nil || len(sessions) == 0 {
		return "Nggak ada tiket aktif saat ini 👍"
	}

	lines := []string{"⏰ Tiket kamu yang aktif:", ""}
	for _, session := range sessions {
		lines = append(lines, s.formatTicketLine(session))
	}
	return strings.Join(lines, "\n")
}

func (s *chatInteractiveServiceImpl) handleBudget(ctx context.Context, issueKey string) string {
	issueKey = strings.TrimSpace(issueKey)
	if issueKey == "" {
		return "Pakai gini: */budget <issueKey>* — contoh: /budget CORE-10765"
	}

	session, found, err := s.issueSessionRepo.GetSessionByIssueKey(ctx, issueKey)
	if err != nil || !found || !session.FlagActive {
		return fmt.Sprintf("Tiket %s tidak ditemukan atau belum/tidak lagi di-track 🤔", issueKey)
	}

	return "📊 " + s.formatTicketLine(*session)
}

// formatTicketLine renders one ticket's SP/elapsed/remaining in the same
// bold-label style as reminder messages (Tech Spec: response text must
// match the existing reminder visual style).
func (s *chatInteractiveServiceImpl) formatTicketLine(session issue_session_dto.IssueSession) string {
	budgetSeconds := int64(session.StoryPoints * 3600)
	elapsedSeconds := issue_session_service.CalculateElapsedSeconds(session, s.businessHours, s.holidayCalendar)
	remainingSeconds := budgetSeconds - elapsedSeconds

	return fmt.Sprintf(
		"*%s* — %s\n*Story Point:* %.1f SP (budget %.1f jam)\n*Waktu berjalan:* %.1f jam\n*Sisa waktu:* %s",
		session.IssueKey, session.IssueSummary,
		session.StoryPoints, float64(budgetSeconds)/3600,
		float64(elapsedSeconds)/3600,
		google_chat_notifier_service.FormatDuration(remainingSeconds),
	)
}

func (s *chatInteractiveServiceImpl) handleOverdue(ctx context.Context) string {
	sessions, err := s.issueSessionRepo.GetActiveSessions(ctx)
	if err != nil {
		return "Nggak ada tiket aktif saat ini 👍"
	}

	var overdue []issue_session_dto.IssueSession
	for _, session := range sessions {
		budgetSeconds := int64(session.StoryPoints * 3600)
		elapsedSeconds := issue_session_service.CalculateElapsedSeconds(session, s.businessHours, s.holidayCalendar)
		if elapsedSeconds > budgetSeconds {
			overdue = append(overdue, session)
		}
	}

	if len(overdue) == 0 {
		return "Semua tiket masih dalam budget 🎉"
	}

	lines := []string{"🔴 Tiket yang sudah lewat budget:", ""}
	for _, session := range overdue {
		lines = append(lines, fmt.Sprintf("*%s* (%s) — %s", session.IssueKey, session.AssigneeEmail, session.IssueSummary))
	}
	return strings.Join(lines, "\n")
}

func (s *chatInteractiveServiceImpl) handleTeam(ctx context.Context) string {
	sessions, err := s.issueSessionRepo.GetActiveSessions(ctx)
	if err != nil || len(sessions) == 0 {
		return "Tidak ada aktivitas Code Process saat ini"
	}

	byAssignee := map[string][]issue_session_dto.IssueSession{}
	var assignees []string
	for _, session := range sessions {
		if _, seen := byAssignee[session.AssigneeEmail]; !seen {
			assignees = append(assignees, session.AssigneeEmail)
		}
		byAssignee[session.AssigneeEmail] = append(byAssignee[session.AssigneeEmail], session)
	}
	sort.Strings(assignees)

	lines := []string{"👥 Ringkasan tim:", ""}
	for _, email := range assignees {
		lines = append(lines, fmt.Sprintf("*%s*", email))
		for _, session := range byAssignee[email] {
			lines = append(lines, fmt.Sprintf("  • %s — %s", session.IssueKey, session.IssueSummary))
		}
	}
	return strings.Join(lines, "\n")
}
