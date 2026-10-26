package registration_service

import (
	"context"
	"fmt"

	"jira-u-bot/modules/jira_client/jira_client_service"
	"jira-u-bot/modules/registration/registration_repository"
)

type registrationServiceImpl struct {
	jiraClient jira_client_service.JiraClientService
	repo       registration_repository.RegistrationRepository
}

func NewRegistrationService(jiraClient jira_client_service.JiraClientService, repo registration_repository.RegistrationRepository) RegistrationService {
	return &registrationServiceImpl{jiraClient: jiraClient, repo: repo}
}

func (s *registrationServiceImpl) Register(ctx context.Context, name, email, googleChatUserID string) error {
	found, err := s.jiraClient.LookupUserByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrJiraLookupUnavailable, err)
	}
	if !found {
		return ErrJiraAccountNotFound
	}

	if err := s.repo.UpsertUser(ctx, name, email, googleChatUserID); err != nil {
		return fmt.Errorf("registration_service: UpsertUser failed: %w", err)
	}
	return nil
}
