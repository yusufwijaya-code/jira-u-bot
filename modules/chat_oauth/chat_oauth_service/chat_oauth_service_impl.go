package chat_oauth_service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"jira-u-bot/config"
)

// Scopes proven sufficient via live testing against the real Google Chat
// API: chat.messages for sending, chat.spaces.create/chat.spaces for
// resolving a DM space by email (spaces:setup).
var oauthScopes = []string{
	"https://www.googleapis.com/auth/chat.messages",
	"https://www.googleapis.com/auth/chat.spaces.create",
	"https://www.googleapis.com/auth/chat.spaces",
}

type chatOAuthServiceImpl struct {
	tokenPath string
	source    oauth2.TokenSource

	mu              sync.Mutex
	lastAccessToken string
}

func NewChatOAuthService(cfg *config.AppConfig) (ChatOAuthService, error) {
	if cfg.GoogleOAuthClientID == "" || cfg.GoogleOAuthClientSecret == "" {
		return nil, nil
	}
	if _, err := os.Stat(cfg.GoogleOAuthTokenPath); err != nil {
		return nil, nil
	}

	tok, err := loadToken(cfg.GoogleOAuthTokenPath)
	if err != nil {
		return nil, err
	}

	oauthConfig := &oauth2.Config{
		ClientID:     cfg.GoogleOAuthClientID,
		ClientSecret: cfg.GoogleOAuthClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       oauthScopes,
	}

	return &chatOAuthServiceImpl{
		tokenPath: cfg.GoogleOAuthTokenPath,
		source:    oauthConfig.TokenSource(context.Background(), tok),
	}, nil
}

// GetAccessToken returns a fresh access token, letting the underlying
// oauth2.TokenSource auto-refresh using the stored refresh token when the
// access token has expired (roughly hourly). Whenever the returned access
// token differs from the last one seen, the full token (including any
// rotated refresh token) is persisted back to disk so a later restart
// doesn't need to refresh again immediately.
func (s *chatOAuthServiceImpl) GetAccessToken(ctx context.Context) (string, error) {
	tok, err := s.source.Token()
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if tok.AccessToken != s.lastAccessToken {
		if err := saveToken(s.tokenPath, tok); err != nil {
			return "", err
		}
		s.lastAccessToken = tok.AccessToken
	}

	return tok.AccessToken, nil
}

func loadToken(path string) (*oauth2.Token, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func saveToken(path string, tok *oauth2.Token) error {
	b, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

// RunSetupFlow runs the one-time interactive OAuth consent flow: it opens a
// local loopback listener, prints a URL for the operator to open and
// authorize in a browser, exchanges the resulting code for a token, and
// saves it to cfg.GoogleOAuthTokenPath. Only ever invoked via the
// `--setup-oauth` CLI flag (main.go) — never during normal service startup.
func RunSetupFlow(cfg *config.AppConfig) error {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.GoogleOAuthClientID,
		ClientSecret: cfg.GoogleOAuthClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       oauthScopes,
		RedirectURL:  "http://localhost:8089/",
	}

	codeCh := make(chan string)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		fmt.Fprintln(w, "Authorization complete — you can close this tab and return to the terminal.")
		go func() { codeCh <- code }()
	})
	srv := &http.Server{Addr: "localhost:8089", Handler: mux}
	go srv.ListenAndServe()

	authURL := oauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Println("Open this URL in your browser, log in with the Google account you want reminders sent from, then click Allow:")
	fmt.Println()
	fmt.Println(authURL)
	fmt.Println()
	fmt.Println("Waiting for you to finish logging in...")

	code := <-codeCh
	srv.Shutdown(context.Background())

	tok, err := oauthConfig.Exchange(context.Background(), code)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.GoogleOAuthTokenPath), 0700); err != nil {
		return fmt.Errorf("failed to create token directory: %w", err)
	}
	if err := saveToken(cfg.GoogleOAuthTokenPath, tok); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	fmt.Println("Success! Token saved to", cfg.GoogleOAuthTokenPath)
	return nil
}
