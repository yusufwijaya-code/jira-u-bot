package admin_dashboard_service

import (
	"context"
	"log"

	"jira-u-bot/modules/admin_dashboard/admin_dashboard_dto"
	"jira-u-bot/modules/chat_dm_notifier/chat_dm_notifier_service"
	"jira-u-bot/modules/jira_credential/jira_credential_service"
	"jira-u-bot/modules/registration/registration_repository"
)

type adminDashboardServiceImpl struct {
	registrationRepo registration_repository.RegistrationRepository
	jiraCredential   jira_credential_service.JiraCredentialService
	dmNotifier       chat_dm_notifier_service.ChatDMNotifierService
	selfEmail        string
}

func NewAdminDashboardService(
	registrationRepo registration_repository.RegistrationRepository,
	jiraCredential jira_credential_service.JiraCredentialService,
	dmNotifier chat_dm_notifier_service.ChatDMNotifierService,
	selfEmail string,
) AdminDashboardService {
	return &adminDashboardServiceImpl{
		registrationRepo: registrationRepo,
		jiraCredential:   jiraCredential,
		dmNotifier:       dmNotifier,
		selfEmail:        selfEmail,
	}
}

func (s *adminDashboardServiceImpl) ListUsers(ctx context.Context) ([]admin_dashboard_dto.UserRow, error) {
	users, err := s.registrationRepo.FindAll(ctx)
	if err != nil {
		return nil, err
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
		rows = append(rows, row)
	}
	return rows, nil
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
