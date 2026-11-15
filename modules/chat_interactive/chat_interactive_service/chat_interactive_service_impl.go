package chat_interactive_service

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sort"
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
)

const registerURL = "https://labcup.tech/nudge/register"

// Command prefixes recognized at the start of a DM message. The /n- prefix
// (operator's choice) keeps these from colliding with any other bot's
// commands in the same space.
const (
	commandMyTickets = "/n-mytickets"
	commandBudget    = "/n-budget"
	commandOverdue   = "/n-overdue"
	commandTeam      = "/n-team"
)

// geserCommandPattern matches /n-geser<ISSUEKEY>:target — e.g.
// /n-geser<OMS-123>:todo or /n-geser<CORE-456>:code-review.
var geserCommandPattern = regexp.MustCompile(`^/n-geser<([A-Za-z]+-\d+)>:(\S+)$`)

// geserTargetStatusNames maps the command's target slug to the Jira status
// name to search for among the issue's available transitions. Matching is
// case-insensitive at use (see handleGeser) because projects spell the
// same status differently — OMS uses "Code Review", CORE uses "CODE REVIEW".
var geserTargetStatusNames = map[string]string{
	"todo":        "To Do",
	"code-review": "Code Review",
}

type chatInteractiveServiceImpl struct {
	registrationRepo registration_repository.RegistrationRepository
	issueSessionRepo issue_session_repository.IssueSessionRepository
	businessHours    business_hours_service.BusinessHoursService
	holidayCalendar  business_hours_dto.CalendarSource
	jiraClient       jira_client_service.JiraClientService
	jiraCredential   jira_credential_service.JiraCredentialService
}

func NewChatInteractiveService(
	registrationRepo registration_repository.RegistrationRepository,
	issueSessionRepo issue_session_repository.IssueSessionRepository,
	businessHours business_hours_service.BusinessHoursService,
	holidayCalendar business_hours_dto.CalendarSource,
	jiraClient jira_client_service.JiraClientService,
	jiraCredential jira_credential_service.JiraCredentialService,
) ChatInteractiveService {
	return &chatInteractiveServiceImpl{
		registrationRepo: registrationRepo,
		issueSessionRepo: issueSessionRepo,
		businessHours:    businessHours,
		holidayCalendar:  holidayCalendar,
		jiraClient:       jiraClient,
		jiraCredential:   jiraCredential,
	}
}

func (s *chatInteractiveServiceImpl) RouteCommand(ctx context.Context, senderEmail, messageText string) string {
	_, found, err := s.registrationRepo.FindUserByEmail(ctx, senderEmail)
	if err != nil || !found {
		return fmt.Sprintf("Kamu belum terdaftar. Daftar dulu di %s ya 🙏", registerURL)
	}

	text := strings.TrimSpace(messageText)
	switch {
	case text == commandMyTickets:
		return s.handleMyTickets(ctx, senderEmail)
	case strings.HasPrefix(text, commandBudget):
		return s.handleBudget(ctx, strings.TrimSpace(strings.TrimPrefix(text, commandBudget)))
	case text == commandOverdue:
		return s.handleOverdue(ctx)
	case text == commandTeam:
		return s.handleTeam(ctx)
	case geserCommandPattern.MatchString(text):
		matches := geserCommandPattern.FindStringSubmatch(text)
		return s.handleGeser(ctx, senderEmail, matches[1], matches[2])
	default:
		return s.handleHelp()
	}
}

// handleGeser implements /n-geser<issueKey>:target — lets the SENDER
// transition one of their own tracked tickets directly, without opening
// Jira. Scope is deliberately limited to tickets the sender is themselves
// assigned to (same ownership check as /n-budget's data, just enforced
// rather than merely displayed) — this command can change real Jira state,
// so it must not let anyone move someone else's ticket.
func (s *chatInteractiveServiceImpl) handleGeser(ctx context.Context, senderEmail, issueKey, targetSlug string) string {
	targetStatusName, recognized := geserTargetStatusNames[strings.ToLower(targetSlug)]
	if !recognized {
		return fmt.Sprintf("Target %q nggak dikenali. Target yang didukung: todo, code-review", targetSlug)
	}

	session, found, err := s.issueSessionRepo.GetSessionByIssueKey(ctx, issueKey)
	if err != nil || !found || !session.FlagActive {
		return fmt.Sprintf("Tiket %s nggak ditemukan atau nggak lagi aktif di-track 🤔", issueKey)
	}
	if session.AssigneeEmail != senderEmail {
		return fmt.Sprintf("Tiket %s bukan tiket kamu — cuma assignee tiket yang bisa geser statusnya 🙏", issueKey)
	}

	// Resolve the sender's personal Jira credential if they've set one up
	// (Tech Spec "Dashboard Jira Manage", AC4.3/AC5.3) — a resolve error
	// (e.g. decrypt failure) falls back to the central credential rather
	// than blocking the transition, same resilience contract as
	// user_dashboard_service.GetMyBoard.
	var cred *jira_client_dto.Credential
	if s.jiraCredential != nil {
		var credErr error
		cred, credErr = s.jiraCredential.ResolveCredential(ctx, senderEmail)
		if credErr != nil {
			log.Printf("chat_interactive_service: ResolveCredential(%s) failed, falling back to central credential: %v", senderEmail, credErr)
		}
	}

	transitions, err := s.jiraClient.GetTransitionsAs(ctx, cred, issueKey)
	if err != nil {
		return fmt.Sprintf("Gagal ambil daftar transisi buat %s dari Jira, coba lagi beberapa saat 🙏", issueKey)
	}

	var matchedID string
	for _, tr := range transitions {
		if strings.EqualFold(tr.To.Name, targetStatusName) {
			matchedID = tr.ID
			break
		}
	}
	if matchedID == "" {
		return fmt.Sprintf("Nggak ada jalur langsung dari status tiket %s sekarang ke %q di workflow project ini 🤔", issueKey, targetStatusName)
	}

	if err := s.jiraClient.TransitionIssueAs(ctx, cred, issueKey, matchedID); err != nil {
		return fmt.Sprintf("Gagal geser %s ke %s, coba lagi beberapa saat 🙏", issueKey, targetStatusName)
	}

	return fmt.Sprintf("✅ *%s* berhasil digeser ke *%s*", issueKey, targetStatusName)
}

func (s *chatInteractiveServiceImpl) handleHelp() string {
	return "🤖 Command yang tersedia:\n\n" +
		"*/n-mytickets* — tiket Code Process kamu\n" +
		"*/n-budget <issueKey>* — detail budget satu tiket\n" +
		"*/n-overdue* — semua tiket yang lewat budget\n" +
		"*/n-team* — ringkasan semua tiket aktif per orang\n" +
		"*/n-geser<issueKey>:todo* — geser tiket kamu balik ke To Do\n" +
		"*/n-geser<issueKey>:code-review* — geser tiket kamu ke Code Review"
}

func (s *chatInteractiveServiceImpl) handleMyTickets(ctx context.Context, email string) string {
	sessions, err := s.issueSessionRepo.FindActiveByAssigneeEmail(ctx, email)
	if err != nil || len(sessions) == 0 {
		return "Nggak ada tiket aktif saat ini 👍"
	}

	lines := []string{"⏰ Tiket kamu yang aktif:", ""}
	for i, session := range sessions {
		if i > 0 {
			lines = append(lines, "") // blank line between cards so they don't visually blend together
		}
		lines = append(lines, s.formatTicketLine(ctx, session))
	}
	return strings.Join(lines, "\n")
}

func (s *chatInteractiveServiceImpl) handleBudget(ctx context.Context, issueKey string) string {
	issueKey = strings.TrimSpace(issueKey)
	if issueKey == "" {
		return "Pakai gini: */n-budget <issueKey>* — contoh: /n-budget CORE-10765"
	}

	session, found, err := s.issueSessionRepo.GetSessionByIssueKey(ctx, issueKey)
	if err != nil || !found || !session.FlagActive {
		return fmt.Sprintf("Tiket %s tidak ditemukan atau belum/tidak lagi di-track 🤔", issueKey)
	}

	return "📊 " + s.formatTicketLine(ctx, *session)
}

// formatTicketLine renders one ticket's SP/elapsed/remaining in the same
// bold-label style as reminder messages.
func (s *chatInteractiveServiceImpl) formatTicketLine(ctx context.Context, session issue_session_dto.IssueSession) string {
	cal := s.holidayCalendar.ForEmail(ctx, session.AssigneeEmail)
	budgetSeconds := int64(session.StoryPoints * 3600)
	elapsedSeconds := issue_session_service.CalculateElapsedSeconds(session, s.businessHours, cal)
	remainingSeconds := budgetSeconds - elapsedSeconds
	deadline := s.businessHours.DeadlineAfterBusinessSeconds(time.Now(), remainingSeconds, cal)

	return fmt.Sprintf(
		"*%s* — %s\n*Story Point:* %.1f SP (budget %.1f jam)\n*Waktu berjalan:* %.1f jam\n*Sisa waktu:* %s\n*Harus Selesai Pass:* %s",
		session.IssueKey, session.IssueSummary,
		session.StoryPoints, float64(budgetSeconds)/3600,
		float64(elapsedSeconds)/3600,
		google_chat_notifier_service.FormatDuration(remainingSeconds),
		google_chat_notifier_service.FormatDeadline(deadline),
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
		elapsedSeconds := issue_session_service.CalculateElapsedSeconds(session, s.businessHours, s.holidayCalendar.ForEmail(ctx, session.AssigneeEmail))
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
