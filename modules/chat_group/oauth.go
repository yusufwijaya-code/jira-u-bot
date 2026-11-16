// Package chat_group adds Nudge users to the Google Group that makes the
// Nudge Chat App visible (Tech Spec "Auto Join Google Group Nudge").
package chat_group

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"nudge/config"
)

// Scope is the only scope the operator grants for this feature.
const Scope = "https://www.googleapis.com/auth/cloud-identity.groups"

// client is the Groups-specific OAuth client, else the Chat operator's one.
// The token must be refreshed with the same client that issued it.
func client(cfg *config.AppConfig) (string, string) {
	if cfg.GoogleGroupsClientID != "" && cfg.GoogleGroupsClientSecret != "" {
		return cfg.GoogleGroupsClientID, cfg.GoogleGroupsClientSecret
	}
	return cfg.GoogleOAuthClientID, cfg.GoogleOAuthClientSecret
}

func oauthConfig(cfg *config.AppConfig, redirect string) *oauth2.Config {
	id, secret := client(cfg)
	return &oauth2.Config{
		ClientID: id, ClientSecret: secret,
		Endpoint: google.Endpoint, Scopes: []string{Scope}, RedirectURL: redirect,
	}
}

// TokenSource returns the operator's Groups token; on=false (no error) when
// the feature is not configured: no group email, no client, or no token file.
func TokenSource(cfg *config.AppConfig) (oauth2.TokenSource, bool, error) {
	if id, secret := client(cfg); cfg.NudgeGroupEmail == "" || id == "" || secret == "" {
		return nil, false, nil
	}
	b, err := os.ReadFile(cfg.GoogleGroupsTokenPath)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return nil, false, fmt.Errorf("chat_group: token file: %w", err)
	}
	return oauthConfig(cfg, "").TokenSource(context.Background(), &tok), true, nil
}

// RunSetupFlow is the one-time consent for `--setup-groups-oauth`: the same
// loopback flow as the Chat token, but its own scope and token file.
func RunSetupFlow(cfg *config.AppConfig) error {
	if id, secret := client(cfg); id == "" || secret == "" {
		return fmt.Errorf("GOOGLE_GROUPS_CLIENT_ID/SECRET (or GOOGLE_OAUTH_CLIENT_ID/SECRET) are required")
	}
	oc := oauthConfig(cfg, "http://localhost:8089/")
	codeCh := make(chan string)
	srv := &http.Server{Addr: "localhost:8089", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Authorization complete — you can close this tab and return to the terminal.")
		go func() { codeCh <- r.URL.Query().Get("code") }()
	})}
	go srv.ListenAndServe()
	fmt.Println("Open this URL, log in as the OWNER of the Nudge group and click Allow:")
	fmt.Println()
	fmt.Println(oc.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce))
	fmt.Println()
	fmt.Println("Waiting for you to finish logging in...")
	code := <-codeCh
	srv.Shutdown(context.Background())
	tok, err := oc.Exchange(context.Background(), code)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.GoogleGroupsTokenPath), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfg.GoogleGroupsTokenPath, b, 0600); err != nil {
		return err
	}
	fmt.Println("Success! Token saved to", cfg.GoogleGroupsTokenPath)
	return nil
}
