package chat_app_activation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"nudge/modules/chat_app/chat_app_client"
)

// OAuth is the consent round-trip of the one-click activation.
type OAuth interface {
	AuthCodeURL(state, loginHint string) string
	// Exchange returns the user's access token and the email it belongs to.
	Exchange(ctx context.Context, code string) (accessToken, email string, err error)
}

type googleOAuth struct {
	cfg *oauth2.Config
}

// NewGoogleOAuth reuses the Sign-In OAuth client; redirectURL must be
// registered on it.
func NewGoogleOAuth(clientID, clientSecret, redirectURL string) OAuth {
	return &googleOAuth{cfg: &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     google.Endpoint,
		Scopes:       []string{"openid", "email", chat_app_client.SpacesCreateScope},
	}}
}

func (g *googleOAuth) AuthCodeURL(state, loginHint string) string {
	return g.cfg.AuthCodeURL(state,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("login_hint", loginHint),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)
}

func (g *googleOAuth) Exchange(ctx context.Context, code string) (string, string, error) {
	tok, err := g.cfg.Exchange(ctx, code)
	if err != nil {
		return "", "", fmt.Errorf("chat_app_activation: exchange: %w", err)
	}
	idToken, _ := tok.Extra("id_token").(string)
	email, err := emailFromIDToken(idToken)
	if err != nil {
		return "", "", err
	}
	return tok.AccessToken, email, nil
}

// emailFromIDToken reads the email claim. The ID token came straight from
// Google's token endpoint over TLS, so its signature needs no re-check.
func emailFromIDToken(idToken string) (string, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return "", errors.New("chat_app_activation: no id_token in the token response")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("chat_app_activation: id_token payload: %w", err)
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Email == "" {
		return "", errors.New("chat_app_activation: id_token has no email")
	}
	return claims.Email, nil
}
