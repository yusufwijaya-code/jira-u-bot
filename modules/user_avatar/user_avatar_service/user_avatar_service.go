// Package user_avatar_service serves users' Google profile pictures through
// the backend (Tech Spec "Admin Panel Revamp" — user avatars). The picture
// URL comes from the Google Sign-In ID token ("picture" claim) and is
// refreshed on every login; the image itself is proxied and cached, so the
// browser never talks to Google directly.
package user_avatar_service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"nudge/modules/user_avatar/user_avatar_repository"
)

// ErrNoAvatar: the user has no stored Google picture (never logged in to
// the dashboard, or Google sent none) — the frontend shows the initial.
var ErrNoAvatar = errors.New("user avatar not available")

const (
	avatarTTL      = 24 * time.Hour
	maxAvatarBytes = 512 * 1024
)

// IsGooglePictureURL reports whether pictureURL is a Google profile picture
// we are willing to fetch: https on *.googleusercontent.com, no userinfo.
func IsGooglePictureURL(pictureURL string) bool {
	u, err := url.Parse(pictureURL)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	return strings.HasSuffix(strings.ToLower(u.Hostname()), ".googleusercontent.com")
}

// ImageFetcher downloads an image — the default fetches over HTTP without
// any credentials.
type ImageFetcher interface {
	Fetch(ctx context.Context, imageURL string) (body []byte, contentType string, err error)
}

type UserAvatarService interface {
	// SavePicture stores userID's Google picture URL from the login's ID
	// token; anything that isn't a Google picture URL is stored as "".
	SavePicture(ctx context.Context, userID int64, pictureURL string) error
	// ByEmail returns the picture of the user with that email.
	ByEmail(ctx context.Context, email string) (body []byte, contentType string, err error)
	// ByUserID returns the picture of the user with that id.
	ByUserID(ctx context.Context, userID int64) (body []byte, contentType string, err error)
}

type cached struct {
	body        []byte
	contentType string
	expires     time.Time
}

type userAvatarServiceImpl struct {
	repo    user_avatar_repository.UserAvatarRepository
	fetcher ImageFetcher
	now     func() time.Time

	mu    sync.Mutex
	cache map[string]cached // by picture URL: a new photo is a new URL
}

func NewUserAvatarService(repo user_avatar_repository.UserAvatarRepository, fetcher ImageFetcher) UserAvatarService {
	if fetcher == nil {
		fetcher = httpImageFetcher{}
	}
	return &userAvatarServiceImpl{repo: repo, fetcher: fetcher, now: time.Now, cache: map[string]cached{}}
}

func (s *userAvatarServiceImpl) SavePicture(ctx context.Context, userID int64, pictureURL string) error {
	if !IsGooglePictureURL(pictureURL) {
		pictureURL = ""
	}
	return s.repo.Save(ctx, userID, pictureURL)
}

func (s *userAvatarServiceImpl) ByEmail(ctx context.Context, email string) ([]byte, string, error) {
	pictureURL, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		return nil, "", err
	}
	return s.picture(ctx, pictureURL)
}

func (s *userAvatarServiceImpl) ByUserID(ctx context.Context, userID int64) ([]byte, string, error) {
	pictureURL, err := s.repo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	return s.picture(ctx, pictureURL)
}

func (s *userAvatarServiceImpl) picture(ctx context.Context, pictureURL string) ([]byte, string, error) {
	// Re-checked here too: never fetch a URL that isn't Google's, whatever
	// ended up in the table.
	if !IsGooglePictureURL(pictureURL) {
		return nil, "", ErrNoAvatar
	}
	s.mu.Lock()
	if c, ok := s.cache[pictureURL]; ok && s.now().Before(c.expires) {
		s.mu.Unlock()
		return c.body, c.contentType, nil
	}
	s.mu.Unlock()

	body, contentType, err := s.fetcher.Fetch(ctx, pictureURL)
	if err != nil {
		return nil, "", err // transient: not cached
	}
	s.mu.Lock()
	s.cache[pictureURL] = cached{body: body, contentType: contentType, expires: s.now().Add(avatarTTL)}
	s.mu.Unlock()
	return body, contentType, nil
}

// httpImageFetcher fetches a Google picture without credentials and only
// follows redirects that stay on Google's picture hosts.
type httpImageFetcher struct{}

func (httpImageFetcher) Fetch(ctx context.Context, imageURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 3 || !IsGooglePictureURL(next.URL.String()) {
				return errors.New("user_avatar: redirect not allowed")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("user_avatar: fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("user_avatar: fetch returned %d", resp.StatusCode)
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("user_avatar: not an image (%s)", contentType)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAvatarBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxAvatarBytes {
		return nil, "", errors.New("user_avatar: image too large")
	}
	return body, contentType, nil
}
