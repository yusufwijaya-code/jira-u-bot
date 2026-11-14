// Package user_avatar_service serves registered users' Jira profile
// pictures through the backend (Tech Spec "Admin Panel Revamp" — user
// avatars): the browser never talks to Jira, and pictures are cached.
package user_avatar_service

import (
	"context"
	"errors"
	"sync"
	"time"

	"nudge/modules/registration/registration_repository"
)

// ErrNoAvatar: the user is unknown, has no Jira account id, or Jira has no
// usable picture for them — the frontend shows the initial instead.
var ErrNoAvatar = errors.New("user avatar not available")

const (
	avatarTTL  = 24 * time.Hour
	missingTTL = time.Hour // don't ask Jira again for every page view
)

// JiraUserClient is jira_client_service.JiraUserClient.
type JiraUserClient interface {
	UserAvatarURL(ctx context.Context, accountID string) (string, error)
	FetchUserAvatar(ctx context.Context, avatarURL string) ([]byte, string, error)
}

type UserAvatarService interface {
	// ByEmail returns the picture of the registered user with that email.
	ByEmail(ctx context.Context, email string) (body []byte, contentType string, err error)
	// ByUserID returns the picture of the registered user with that id.
	ByUserID(ctx context.Context, userID int64) (body []byte, contentType string, err error)
}

type cached struct {
	body        []byte
	contentType string
	missing     bool
	expires     time.Time
}

type userAvatarServiceImpl struct {
	users registration_repository.RegistrationRepository
	jira  JiraUserClient
	now   func() time.Time

	mu    sync.Mutex
	cache map[string]cached // by Jira account id
}

func NewUserAvatarService(users registration_repository.RegistrationRepository, jira JiraUserClient) UserAvatarService {
	return &userAvatarServiceImpl{users: users, jira: jira, now: time.Now, cache: map[string]cached{}}
}

func (s *userAvatarServiceImpl) ByEmail(ctx context.Context, email string) ([]byte, string, error) {
	user, found, err := s.users.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, "", err
	}
	if !found {
		return nil, "", ErrNoAvatar
	}
	return s.byAccount(ctx, user.JiraAccountID)
}

func (s *userAvatarServiceImpl) ByUserID(ctx context.Context, userID int64) ([]byte, string, error) {
	users, err := s.users.FindAll(ctx)
	if err != nil {
		return nil, "", err
	}
	for _, u := range users {
		if u.UserID == userID {
			return s.byAccount(ctx, u.JiraAccountID)
		}
	}
	return nil, "", ErrNoAvatar
}

func (s *userAvatarServiceImpl) byAccount(ctx context.Context, accountID string) ([]byte, string, error) {
	if accountID == "" {
		return nil, "", ErrNoAvatar
	}
	s.mu.Lock()
	if c, ok := s.cache[accountID]; ok && s.now().Before(c.expires) {
		s.mu.Unlock()
		if c.missing {
			return nil, "", ErrNoAvatar
		}
		return c.body, c.contentType, nil
	}
	s.mu.Unlock()

	body, contentType, err := s.fetch(ctx, accountID)
	if errors.Is(err, ErrNoAvatar) {
		s.store(accountID, cached{missing: true, expires: s.now().Add(missingTTL)})
		return nil, "", err
	}
	if err != nil {
		return nil, "", err // transient: not cached
	}
	s.store(accountID, cached{body: body, contentType: contentType, expires: s.now().Add(avatarTTL)})
	return body, contentType, nil
}

func (s *userAvatarServiceImpl) fetch(ctx context.Context, accountID string) ([]byte, string, error) {
	avatarURL, err := s.jira.UserAvatarURL(ctx, accountID)
	if err != nil {
		return nil, "", err
	}
	if avatarURL == "" {
		return nil, "", ErrNoAvatar
	}
	body, contentType, err := s.jira.FetchUserAvatar(ctx, avatarURL)
	if err != nil {
		return nil, "", err
	}
	return body, contentType, nil
}

func (s *userAvatarServiceImpl) store(accountID string, c cached) {
	s.mu.Lock()
	s.cache[accountID] = c
	s.mu.Unlock()
}
