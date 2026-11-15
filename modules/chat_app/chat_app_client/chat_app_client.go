// Package chat_app_client talks to the Google Chat API as the Nudge Chat
// App (service account, scope chat.bot), plus the one user-token call the
// one-click activation needs (Tech Spec "Nudge sebagai Google Chat App").
package chat_app_client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"nudge/base/helpers/curl_helper"
	"nudge/modules/chat_app/chat_app_dto"
)

// chatAPIBase is a var only so tests can point it at an httptest.Server.
var chatAPIBase = "https://chat.googleapis.com/v1"

const (
	BotScope          = "https://www.googleapis.com/auth/chat.bot"
	SpacesCreateScope = "https://www.googleapis.com/auth/chat.spaces.create"
	requestTimeoutMs  = 10000
)

// Client sends messages as the Nudge Chat App.
type Client interface {
	CreateMessage(ctx context.Context, space string, msg chat_app_dto.OutgoingMessage) error
}

type client struct {
	tokens oauth2.TokenSource
}

func New(tokens oauth2.TokenSource) Client {
	return &client{tokens: tokens}
}

// NewFromFile builds a Client from the service account JSON key at path.
func NewFromFile(path string) (Client, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("chat_app_client: read credentials %s: %w", path, err)
	}
	cfg, err := google.JWTConfigFromJSON(key, BotScope)
	if err != nil {
		return nil, fmt.Errorf("chat_app_client: parse credentials: %w", err)
	}
	return New(cfg.TokenSource(context.Background())), nil
}

type createMessageBody struct {
	Text         string                    `json:"text,omitempty"`
	CardsV2      []chat_app_dto.CardWithID `json:"cardsV2,omitempty"`
	MarkupSyntax string                    `json:"markupSyntax"`
}

func (c *client) CreateMessage(ctx context.Context, space string, msg chat_app_dto.OutgoingMessage) error {
	tok, err := c.tokens.Token()
	if err != nil {
		return fmt.Errorf("chat_app_client: token: %w", err)
	}
	// Same markup as the operator DM path (*bold*, single \n line breaks).
	body := createMessageBody{Text: msg.Text, CardsV2: msg.CardsV2, MarkupSyntax: "MARKUP_SYNTAX_CHAT"}
	_, err = curl_helper.CurlRequest[struct{}](
		ctx, chatAPIBase+"/"+space+"/messages",
		map[string]string{"Authorization": "Bearer " + tok.AccessToken},
		body, http.MethodPost, requestTimeoutMs, false,
	)
	if err != nil {
		return fmt.Errorf("chat_app_client: create message in %s: %w", space, err)
	}
	return nil
}

type setupResponse struct {
	Name string `json:"name"`
}

// SetupBotDM creates (or returns the existing) DM between the user who owns
// userAccessToken and the calling Chat App — i.e. installs the app for them.
func SetupBotDM(ctx context.Context, userAccessToken string) (string, error) {
	body := map[string]any{"space": map[string]any{"spaceType": "DIRECT_MESSAGE", "singleUserBotDm": true}}
	res, err := curl_helper.CurlRequest[setupResponse](
		ctx, chatAPIBase+"/spaces:setup",
		map[string]string{"Authorization": "Bearer " + userAccessToken},
		body, http.MethodPost, requestTimeoutMs, true,
	)
	if err != nil {
		return "", fmt.Errorf("chat_app_client: spaces:setup: %w", err)
	}
	if res == nil || res.Name == "" {
		return "", errors.New("chat_app_client: spaces:setup returned no space")
	}
	return res.Name, nil
}

// IsGone reports a 403/404 from the Chat API: the app was removed from the
// DM, or the user is no longer allowed to use it.
func IsGone(err error) bool {
	var httpErr *curl_helper.HTTPError
	return errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusForbidden || httpErr.StatusCode == http.StatusNotFound)
}
