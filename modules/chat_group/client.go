package chat_group

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

const DefaultBaseURL = "https://cloudidentity.googleapis.com/v1"

var (
	// ErrUnauthorized: the operator token is invalid or revoked (HTTP 401).
	ErrUnauthorized = errors.New("chat_group: unauthorized")
	// ErrGroupConfig: the group can't be looked up (wrong NUDGE_GROUP_EMAIL,
	// or the operator isn't its owner/manager) — a setup problem, not the user's.
	ErrGroupConfig = errors.New("chat_group: group not accessible")
)

// Client adds members to one Google Group through the Cloud Identity Groups API.
type Client struct {
	http  *http.Client
	base  string
	group string

	mu   sync.Mutex
	name string // "groups/{id}", cached after the first lookup
}

func NewClient(ts oauth2.TokenSource, groupEmail, baseURL string) *Client {
	hc := oauth2.NewClient(context.Background(), ts)
	hc.Timeout = 15 * time.Second
	return &Client{http: hc, base: baseURL, group: groupEmail}
}

func (c *Client) do(ctx context.Context, method, u string, body any) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		// oauth2 reports a refused refresh token as a transport error; only a
		// revoked/invalid grant means the token is dead (5xx/429 are transient).
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && (re.ErrorCode == "invalid_grant" || re.ErrorCode == "invalid_client" || re.ErrorCode == "unauthorized_client") {
			return http.StatusUnauthorized, nil, nil
		}
		return 0, nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	return res.StatusCode, b, nil
}

func (c *Client) groupName(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.name != "" {
		return c.name, nil
	}
	status, body, err := c.do(ctx, http.MethodGet, c.base+"/groups:lookup?groupKey.id="+url.QueryEscape(c.group), nil)
	if err != nil {
		return "", err
	}
	if status == http.StatusUnauthorized {
		return "", ErrUnauthorized
	}
	if status == http.StatusForbidden || status == http.StatusNotFound {
		return "", fmt.Errorf("%w: lookup %s: HTTP %d: %s", ErrGroupConfig, c.group, status, body)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("chat_group: lookup %s: HTTP %d: %s", c.group, status, body)
	}
	var out struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Name == "" {
		return "", fmt.Errorf("chat_group: lookup %s: unexpected response %s", c.group, body)
	}
	c.name = out.Name
	return c.name, nil
}

// AddMember adds email as a MEMBER; already being a member (409) is success.
func (c *Client) AddMember(ctx context.Context, email string) error {
	name, err := c.groupName(ctx)
	if err != nil {
		return err
	}
	body := map[string]any{
		"preferredMemberKey": map[string]string{"id": email},
		"roles":              []map[string]string{{"name": "MEMBER"}},
	}
	status, resBody, err := c.do(ctx, http.MethodPost, c.base+"/"+name+"/memberships", body)
	switch {
	case err != nil:
		return err
	case status == http.StatusOK || status == http.StatusConflict:
		return nil
	case status == http.StatusUnauthorized:
		return ErrUnauthorized
	}
	return fmt.Errorf("chat_group: add %s: HTTP %d: %s", email, status, resBody)
}
