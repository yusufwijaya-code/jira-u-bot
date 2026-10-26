package chat_oauth_service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"jira-u-bot/config"
)

func TestNewChatOAuthService_ReturnsNilWhenClientCredentialsNotConfigured(t *testing.T) {
	cfg := &config.AppConfig{GoogleOAuthTokenPath: filepath.Join(t.TempDir(), "token.json")}

	svc, err := NewChatOAuthService(cfg)
	if err != nil {
		t.Fatalf("NewChatOAuthService returned error: %v, want nil error", err)
	}
	if svc != nil {
		t.Error("expected nil service when GOOGLE_OAUTH_CLIENT_ID/SECRET are unset (spec AC2.3 — DM feature inactive, not a startup failure)")
	}
}

func TestNewChatOAuthService_ReturnsNilWhenTokenFileDoesNotExist(t *testing.T) {
	cfg := &config.AppConfig{
		GoogleOAuthClientID:     "test-client-id",
		GoogleOAuthClientSecret: "test-client-secret",
		GoogleOAuthTokenPath:    filepath.Join(t.TempDir(), "does-not-exist.json"),
	}

	svc, err := NewChatOAuthService(cfg)
	if err != nil {
		t.Fatalf("NewChatOAuthService returned error: %v, want nil error", err)
	}
	if svc != nil {
		t.Error("expected nil service when the token file hasn't been created yet (--setup-oauth never run)")
	}
}

func writeTestToken(t *testing.T, path string, tok oauth2.Token) {
	t.Helper()
	b, err := json.Marshal(tok)
	if err != nil {
		t.Fatalf("failed to marshal test token: %v", err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatalf("failed to write test token file: %v", err)
	}
}

func TestGetAccessToken_ReturnsStoredTokenWithoutNetworkCallWhenStillValid(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token.json")
	// Far-future expiry means oauth2's internal ReuseTokenSource sees the
	// token as still Valid() and returns it directly — no HTTP refresh
	// call is made, so this test needs no network access or mock server.
	writeTestToken(t, tokenPath, oauth2.Token{
		AccessToken:  "fake-access-token",
		RefreshToken: "fake-refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(1 * time.Hour),
	})

	cfg := &config.AppConfig{
		GoogleOAuthClientID:     "test-client-id",
		GoogleOAuthClientSecret: "test-client-secret",
		GoogleOAuthTokenPath:    tokenPath,
	}

	svc, err := NewChatOAuthService(cfg)
	if err != nil {
		t.Fatalf("NewChatOAuthService returned error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected a non-nil service when client credentials and a token file are both present")
	}

	token, err := svc.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("GetAccessToken returned error: %v", err)
	}
	if token != "fake-access-token" {
		t.Errorf("GetAccessToken = %q, want %q", token, "fake-access-token")
	}
}
