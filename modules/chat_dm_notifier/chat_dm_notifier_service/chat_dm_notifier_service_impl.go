package chat_dm_notifier_service

import (
	"context"
	"fmt"
	"net/http"

	"jira-u-bot/base/helpers/curl_helper"
	"jira-u-bot/modules/chat_oauth/chat_oauth_service"
	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_dto"
	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_service"
	"jira-u-bot/modules/registration/registration_repository"
)

// chatAPIBase is a package-level var (not a const) so tests can point it at
// an httptest.Server — the Google Chat API base URL is never meant to be
// operator-configurable in production, so this is purely a test seam, not
// a new config knob.
var chatAPIBase = "https://chat.googleapis.com/v1"

type chatDMNotifierServiceImpl struct {
	oauth            chat_oauth_service.ChatOAuthService
	registrationRepo registration_repository.RegistrationRepository
}

func NewChatDMNotifierService(oauth chat_oauth_service.ChatOAuthService, registrationRepo registration_repository.RegistrationRepository) ChatDMNotifierService {
	return &chatDMNotifierServiceImpl{oauth: oauth, registrationRepo: registrationRepo}
}

type setupSpaceRequest struct {
	Space       setupSpaceSpace        `json:"space"`
	Memberships []setupSpaceMembership `json:"memberships"`
}
type setupSpaceSpace struct {
	SpaceType string `json:"spaceType"`
}
type setupSpaceMembership struct {
	Member setupSpaceMember `json:"member"`
}
type setupSpaceMember struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type setupSpaceResponse struct {
	Name string `json:"name"`
}

// resolveDMSpace returns the Google Chat DM space resource name for email,
// using the cached value on ms_user.googleChatDMSpaceID when present (spec
// AC2.4) or resolving it once via spaces:setup (confirmed live-tested:
// DM targeting resolves directly by email, no numeric Google Chat user ID
// needed) and caching the result. spaces:setup is idempotent — safe to
// call again if the cache is ever wrong/stale.
func (s *chatDMNotifierServiceImpl) resolveDMSpace(ctx context.Context, email string) (string, error) {
	user, found, err := s.registrationRepo.FindUserByEmail(ctx, email)
	if err != nil {
		return "", fmt.Errorf("chat_dm_notifier: FindUserByEmail(%s) failed: %w", email, err)
	}
	if found && user.GoogleChatDMSpaceID != "" {
		return user.GoogleChatDMSpaceID, nil
	}

	token, err := s.oauth.GetAccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("chat_dm_notifier: GetAccessToken failed: %w", err)
	}

	reqBody := setupSpaceRequest{
		Space: setupSpaceSpace{SpaceType: "DIRECT_MESSAGE"},
		Memberships: []setupSpaceMembership{
			{Member: setupSpaceMember{Name: "users/" + email, Type: "HUMAN"}},
		},
	}

	result, err := curl_helper.CurlRequest[setupSpaceResponse](
		ctx, chatAPIBase+"/spaces:setup",
		map[string]string{"Authorization": "Bearer " + token},
		reqBody, http.MethodPost, 10000, true,
	)
	if err != nil {
		return "", fmt.Errorf("chat_dm_notifier: spaces:setup(%s) failed: %w", email, err)
	}

	if err := s.registrationRepo.UpdateGoogleChatDMSpaceID(ctx, email, result.Name); err != nil {
		return "", fmt.Errorf("chat_dm_notifier: UpdateGoogleChatDMSpaceID(%s) failed: %w", email, err)
	}

	return result.Name, nil
}

func (s *chatDMNotifierServiceImpl) SendReminder(ctx context.Context, msg google_chat_notifier_dto.ReminderMessage) error {
	spaceName, err := s.resolveDMSpace(ctx, msg.AssigneeEmail)
	if err != nil {
		return err
	}
	return s.postMessage(ctx, spaceName, google_chat_notifier_service.BuildMessageText(msg))
}

func (s *chatDMNotifierServiceImpl) SendPlainTextDM(ctx context.Context, email, text string) error {
	spaceName, err := s.resolveDMSpace(ctx, email)
	if err != nil {
		return err
	}
	return s.postMessage(ctx, spaceName, text)
}

func (s *chatDMNotifierServiceImpl) postMessage(ctx context.Context, spaceName, text string) error {
	token, err := s.oauth.GetAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("chat_dm_notifier: GetAccessToken failed: %w", err)
	}

	// markupSyntax must be forced to MARKUP_SYNTAX_CHAT — spaces.messages.create
	// defaults to MARKUP_SYNTAX_MARKDOWN (CommonMark) when unset, under which a
	// single *word* renders as italic (not bold, as under Chat markup syntax)
	// and a lone \n is a soft break instead of a hard line break. The webhook
	// path (google_chat_notifier_service) never hits this because incoming
	// webhooks always use Chat markup syntax; BuildMessageText's *bold* labels
	// and single-\n line breaks only need this to also hold here.
	payload := map[string]string{"text": text, "markupSyntax": "MARKUP_SYNTAX_CHAT"}
	_, err = curl_helper.CurlRequest[struct{}](
		ctx, chatAPIBase+"/"+spaceName+"/messages",
		map[string]string{"Authorization": "Bearer " + token},
		payload, http.MethodPost, 10000, false,
	)
	if err != nil {
		return fmt.Errorf("chat_dm_notifier: post message to %s failed: %w", spaceName, err)
	}
	return nil
}
