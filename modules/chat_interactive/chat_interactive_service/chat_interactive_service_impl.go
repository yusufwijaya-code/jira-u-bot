package chat_interactive_service

import (
	"context"
	"fmt"

	"jira-u-bot/modules/business_hours/business_hours_dto"
	"jira-u-bot/modules/business_hours/business_hours_service"
	"jira-u-bot/modules/chat_interactive/chat_interactive_dto"
	"jira-u-bot/modules/issue_session/issue_session_repository"
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
	return "" // replaced in Task 6
}

func (s *chatInteractiveServiceImpl) handleBudget(ctx context.Context, issueKey string) string {
	return "" // replaced in Task 6
}

func (s *chatInteractiveServiceImpl) handleOverdue(ctx context.Context) string {
	return "" // replaced in Task 7
}

func (s *chatInteractiveServiceImpl) handleTeam(ctx context.Context) string {
	return "" // replaced in Task 7
}
