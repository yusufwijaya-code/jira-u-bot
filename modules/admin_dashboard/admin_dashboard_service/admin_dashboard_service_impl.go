package admin_dashboard_service

import (
	"context"
	"log"
	"strings"

	"nudge/modules/admin_dashboard/admin_dashboard_dto"
	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/chat_delivery"
	"nudge/modules/chat_dm_notifier/chat_dm_notifier_service"
	"nudge/modules/jira_credential/jira_credential_service"
	"nudge/modules/registration/registration_repository"
)

type adminDashboardServiceImpl struct {
	registrationRepo registration_repository.RegistrationRepository
	jiraCredential   jira_credential_service.JiraCredentialService
	dmNotifier       chat_dm_notifier_service.ChatDMNotifierService
	selfEmail        string
	boards           BoardNamer
	chatApp          ChatAppStates
	chatAppEnabled   bool
	// sendSync makes NotifyUserChanged send inline (tests only).
	sendSync bool
}

func NewAdminDashboardService(
	registrationRepo registration_repository.RegistrationRepository,
	jiraCredential jira_credential_service.JiraCredentialService,
	dmNotifier chat_dm_notifier_service.ChatDMNotifierService,
	selfEmail string,
	opts ...Option,
) AdminDashboardService {
	s := &adminDashboardServiceImpl{
		registrationRepo: registrationRepo,
		jiraCredential:   jiraCredential,
		dmNotifier:       dmNotifier,
		selfEmail:        selfEmail,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *adminDashboardServiceImpl) ListUsers(ctx context.Context) ([]admin_dashboard_dto.UserRow, error) {
	users, err := s.registrationRepo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	// Chat App state is best-effort too: on error the rows just show
	// "not activated" with the automatic sender.
	var chatStates map[string]chat_app_dto.State
	if s.chatApp != nil {
		if m, err := s.chatApp.ListAll(ctx); err == nil {
			chatStates = m
		}
	}

	rows := make([]admin_dashboard_dto.UserRow, 0, len(users))
	for _, u := range users {
		row := admin_dashboard_dto.UserRow{
			UserID:        u.UserID,
			Email:         u.Email,
			UserName:      u.UserName,
			FlagActive:    u.FlagActive,
			CreatedDate:   u.CreatedDate,
			JiraAccountID: u.JiraAccountID,
		}
		// Credential status is best-effort: a lookup error (e.g. the
		// encryption key being unconfigured) must not break the whole
		// list, it just shows as "no credential" for that row.
		if preview, found, err := s.jiraCredential.MaskedPreview(ctx, u.UserID); err == nil && found {
			row.HasCredential = true
			row.CredentialMasked = preview
		}
		st := chatStates[strings.ToLower(u.Email)]
		row.ChatAppActivated = st.Activated()
		row.ChatAppActivatedDate = st.ActivatedDate
		row.ChatSender = st.Sender
		row.EffectiveChatSender = chat_delivery.EffectiveSender(st, chat_delivery.IsOperator(u.Email, s.selfEmail), s.chatAppEnabled)
		rows = append(rows, row)
	}
	return rows, nil
}

// GetUser filters ListUsers instead of adding a repository query — the
// registered user base is small and this keeps the credential status logic
// in one place.
func (s *adminDashboardServiceImpl) GetUser(ctx context.Context, userID int64) (admin_dashboard_dto.UserRow, error) {
	rows, err := s.ListUsers(ctx)
	if err != nil {
		return admin_dashboard_dto.UserRow{}, err
	}
	for _, r := range rows {
		if r.UserID == userID {
			return r, nil
		}
	}
	return admin_dashboard_dto.UserRow{}, ErrUserNotFound
}

func (s *adminDashboardServiceImpl) SetActive(ctx context.Context, actingUserID, targetUserID int64, active bool) error {
	if actingUserID == targetUserID {
		return ErrCannotModifySelf
	}
	return s.registrationRepo.UpdateFlagActive(ctx, targetUserID, active)
}

func (s *adminDashboardServiceImpl) UpdateJiraAccountID(ctx context.Context, targetUserID int64, newAccountID string) error {
	return s.registrationRepo.UpdateJiraAccountID(ctx, targetUserID, newAccountID)
}

func (s *adminDashboardServiceImpl) DeleteUser(ctx context.Context, actingUserID, targetUserID int64) error {
	if actingUserID == targetUserID {
		return ErrCannotModifySelf
	}
	return s.registrationRepo.DeleteByID(ctx, targetUserID)
}

func (s *adminDashboardServiceImpl) DeleteUserCredential(ctx context.Context, targetUserID int64) error {
	return s.jiraCredential.DeleteCredential(ctx, targetUserID)
}

func (s *adminDashboardServiceImpl) BroadcastMessage(ctx context.Context, message string) (admin_dashboard_dto.BroadcastResult, error) {
	if s.dmNotifier == nil {
		return admin_dashboard_dto.BroadcastResult{}, ErrDMNotConfigured
	}

	users, err := s.registrationRepo.FindAll(ctx)
	if err != nil {
		return admin_dashboard_dto.BroadcastResult{}, err
	}

	var result admin_dashboard_dto.BroadcastResult
	for _, u := range users {
		if !u.FlagActive || u.Email == s.selfEmail {
			continue
		}
		if err := s.dmNotifier.SendPlainTextDM(ctx, u.Email, message); err != nil {
			log.Printf("admin_dashboard_service: broadcast DM to %s failed: %v", u.Email, err)
			result.FailedEmails = append(result.FailedEmails, u.Email)
			continue
		}
		result.SentCount++
	}
	return result, nil
}
