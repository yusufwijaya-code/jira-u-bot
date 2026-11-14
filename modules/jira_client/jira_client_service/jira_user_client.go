package jira_client_service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nudge/base/helpers/curl_helper"
	"nudge/config"
)

// JiraUserClient reads a Jira user's profile picture (Tech Spec "Admin Panel
// Revamp" — user avatars). Separate from JiraAgileClient so that interface
// and its test fakes stay unchanged.
type JiraUserClient interface {
	// UserAvatarURL returns the 48x48 avatar URL of accountID ("" if none).
	UserAvatarURL(ctx context.Context, accountID string) (string, error)
	// FetchUserAvatar downloads a user avatar. Jira-site URLs are fetched
	// with the bot credentials; Atlassian's avatar CDN and Gravatar (where
	// Jira Cloud hosts user pictures) are fetched WITHOUT credentials; any
	// other host — including a redirect to one — is refused.
	FetchUserAvatar(ctx context.Context, avatarURL string) (body []byte, contentType string, err error)
}

func NewJiraUserClient(cfg *config.AppConfig) JiraUserClient {
	return &jiraClientServiceImpl{cfg: cfg}
}

// ErrAvatarHostNotAllowed: the avatar URL is not on a host we proxy.
var ErrAvatarHostNotAllowed = errors.New("jira_client: avatar host not allowed")

type jiraUserResponse struct {
	AvatarURLs map[string]string `json:"avatarUrls"`
}

func (s *jiraClientServiceImpl) UserAvatarURL(ctx context.Context, accountID string) (string, error) {
	reqURL := fmt.Sprintf("%s/rest/api/3/user?accountId=%s", s.cfg.JiraBaseURL, url.QueryEscape(accountID))
	result, err := curl_helper.CurlRequest[jiraUserResponse](ctx, reqURL, s.authHeaders(), nil, http.MethodGet, 10000, true)
	if err != nil {
		return "", fmt.Errorf("jira_client: UserAvatarURL(%s) failed: %w", accountID, err)
	}
	return result.AvatarURLs["48x48"], nil
}

// avatarAccess says whether avatarURL may be proxied and whether it gets
// the bot credentials (only on the configured Jira site itself).
func avatarAccess(jiraBaseURL, avatarURL string) (allowed, withAuth bool) {
	base := strings.TrimRight(jiraBaseURL, "/")
	if base != "" && strings.HasPrefix(avatarURL, base+"/") {
		return true, true
	}
	u, err := url.Parse(avatarURL)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false, false
	}
	host := strings.ToLower(u.Hostname())
	if host == "secure.gravatar.com" || strings.HasSuffix(host, ".atl-paas.net") {
		return true, false
	}
	return false, false
}

func (s *jiraClientServiceImpl) FetchUserAvatar(ctx context.Context, avatarURL string) ([]byte, string, error) {
	allowed, withAuth := avatarAccess(s.cfg.JiraBaseURL, avatarURL)
	if !allowed {
		return nil, "", ErrAvatarHostNotAllowed
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, avatarURL, nil)
	if err != nil {
		return nil, "", err
	}
	if withAuth {
		req.Header.Set("Authorization", basicAuthHeader(s.cfg.JiraEmail, s.cfg.JiraAPIToken))
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		// Gravatar redirects to Atlassian's CDN for its default picture:
		// follow only to allowed hosts, and never carry the credentials.
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("jira_client: too many avatar redirects")
			}
			ok, auth := avatarAccess(s.cfg.JiraBaseURL, next.URL.String())
			if !ok {
				return ErrAvatarHostNotAllowed
			}
			if !auth {
				next.Header.Del("Authorization")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("jira_client: FetchUserAvatar failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("jira_client: FetchUserAvatar returned %d", resp.StatusCode)
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("jira_client: avatar is not an image (%s)", contentType)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAvatarBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxAvatarBytes {
		return nil, "", fmt.Errorf("jira_client: avatar too large")
	}
	return body, contentType, nil
}
