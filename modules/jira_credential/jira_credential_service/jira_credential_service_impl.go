package jira_credential_service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"nudge/config"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_credential/jira_credential_crypto"
	"nudge/modules/jira_credential/jira_credential_repository"
	"nudge/modules/registration/registration_repository"
)

// ErrEncryptionKeyNotConfigured is returned by UpsertCredential when
// CREDENTIAL_ENCRYPTION_KEY isn't set (or isn't a valid 32-byte base64
// key) — the service still starts up in this state (spec: optional
// feature, same graceful-degradation precedent as chat_oauth_service
// returning a nil service rather than failing boot), it just can't accept
// new personal credentials until the key is fixed.
var ErrEncryptionKeyNotConfigured = errors.New("jira_credential: CREDENTIAL_ENCRYPTION_KEY is not configured")

// previewSuffixLen is how many trailing characters of the decrypted token
// MaskedPreview reveals (spec AC6.4) — enough to help an admin recognize
// "yes that's the token I rotated", not enough to reconstruct it.
const previewSuffixLen = 4

type jiraCredentialServiceImpl struct {
	repo             jira_credential_repository.JiraCredentialRepository
	registrationRepo registration_repository.RegistrationRepository
	key              [32]byte
	keyConfigured    bool
}

func NewJiraCredentialService(
	cfg *config.AppConfig,
	repo jira_credential_repository.JiraCredentialRepository,
	registrationRepo registration_repository.RegistrationRepository,
) JiraCredentialService {
	key, ok := parseKey(cfg.CredentialEncryptionKey)
	return &jiraCredentialServiceImpl{repo: repo, registrationRepo: registrationRepo, key: key, keyConfigured: ok}
}

func parseKey(encoded string) ([32]byte, bool) {
	var key [32]byte
	if encoded == "" {
		return key, false
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != 32 {
		return key, false
	}
	copy(key[:], decoded)
	return key, true
}

func (s *jiraCredentialServiceImpl) ResolveCredential(ctx context.Context, email string) (*jira_client_dto.Credential, error) {
	user, found, err := s.registrationRepo.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	record, found, err := s.repo.FindByUserID(ctx, user.UserID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	if !s.keyConfigured {
		return nil, fmt.Errorf("jira_credential: cannot decrypt credential for user %d: %w", user.UserID, ErrEncryptionKeyNotConfigured)
	}
	plaintext, err := jira_credential_crypto.Decrypt(record.JiraAPITokenEncrypted, s.key)
	if err != nil {
		return nil, fmt.Errorf("jira_credential: decrypt failed for user %d: %w", user.UserID, err)
	}

	return &jira_client_dto.Credential{Email: record.JiraEmail, APIToken: string(plaintext)}, nil
}

func (s *jiraCredentialServiceImpl) UpsertCredential(ctx context.Context, userID int64, jiraEmail, apiToken string) error {
	if !s.keyConfigured {
		return ErrEncryptionKeyNotConfigured
	}
	encrypted, err := jira_credential_crypto.Encrypt([]byte(apiToken), s.key)
	if err != nil {
		return err
	}
	return s.repo.Upsert(ctx, userID, jiraEmail, encrypted)
}

func (s *jiraCredentialServiceImpl) DeleteCredential(ctx context.Context, userID int64) error {
	return s.repo.DeleteByUserID(ctx, userID)
}

func (s *jiraCredentialServiceImpl) MaskedPreview(ctx context.Context, userID int64) (string, bool, error) {
	record, found, err := s.repo.FindByUserID(ctx, userID)
	if err != nil || !found {
		return "", found, err
	}
	if !s.keyConfigured {
		return "", true, ErrEncryptionKeyNotConfigured
	}

	plaintext, err := jira_credential_crypto.Decrypt(record.JiraAPITokenEncrypted, s.key)
	if err != nil {
		return "", true, err
	}

	suffix := string(plaintext)
	if len(suffix) > previewSuffixLen {
		suffix = suffix[len(suffix)-previewSuffixLen:]
	}
	return "••••••••" + suffix, true, nil
}
