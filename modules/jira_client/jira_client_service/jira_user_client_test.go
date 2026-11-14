package jira_client_service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"nudge/config"
)

func TestAvatarAccess(t *testing.T) {
	const jira = "https://esb.atlassian.net"
	cases := []struct {
		url               string
		allowed, withAuth bool
	}{
		{"https://esb.atlassian.net/secure/useravatar?avatarId=1", true, true},
		{"https://avatar-management--avatars.us-west-2.prod.public.atl-paas.net/a/48", true, false},
		{"https://secure.gravatar.com/avatar/abc?d=x", true, false},
		{"http://secure.gravatar.com/avatar/abc", false, false},          // not https
		{"https://evil.com/atl-paas.net/x", false, false},                // suffix must be the host
		{"https://atl-paas.net.evil.com/x", false, false},                // look-alike host
		{"https://user:pw@secure.gravatar.com/avatar/abc", false, false}, // userinfo
		{"https://esb.atlassian.net.evil.com/x", false, false},           // look-alike Jira site
		{"ftp://secure.gravatar.com/x", false, false},
	}
	for _, tc := range cases {
		allowed, withAuth := avatarAccess(jira, tc.url)
		if allowed != tc.allowed || withAuth != tc.withAuth {
			t.Errorf("%s: allowed=%v withAuth=%v, want %v/%v", tc.url, allowed, withAuth, tc.allowed, tc.withAuth)
		}
	}
}

func TestFetchUserAvatar_RefusesOtherHosts(t *testing.T) {
	c := NewJiraUserClient(&config.AppConfig{JiraBaseURL: "https://esb.atlassian.net"})
	if _, _, err := c.FetchUserAvatar(context.Background(), "https://evil.com/a.png"); !errors.Is(err, ErrAvatarHostNotAllowed) {
		t.Errorf("err = %v, want ErrAvatarHostNotAllowed", err)
	}
}

func TestFetchUserAvatar_JiraSiteUsesBotAuthAndChecksImage(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path == "/text" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("PNG"))
	}))
	defer srv.Close()
	c := NewJiraUserClient(&config.AppConfig{JiraBaseURL: srv.URL, JiraEmail: "bot@esb.co.id", JiraAPIToken: "tok"})

	body, ct, err := c.FetchUserAvatar(context.Background(), srv.URL+"/avatar.png")
	if err != nil || string(body) != "PNG" || ct != "image/png" {
		t.Fatalf("body=%q ct=%q err=%v", body, ct, err)
	}
	if gotAuth == "" {
		t.Error("Jira-site avatar must be fetched with the bot credentials")
	}
	if _, _, err := c.FetchUserAvatar(context.Background(), srv.URL+"/text"); err == nil {
		t.Error("non-image response must be rejected")
	}
}
