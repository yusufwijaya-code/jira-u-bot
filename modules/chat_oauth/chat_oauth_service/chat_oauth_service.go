package chat_oauth_service

import "context"

// ChatOAuthService owns the lifecycle of the OAuth token used to send
// Google Chat messages as Yusuf's own account (spec Background — this is
// a real end-user OAuth grant, not a service-account/domain-wide-delegation
// Chat App). NewChatOAuthService returns a nil service (not an error) when
// OAuth isn't configured yet — callers must handle a nil ChatOAuthService
// by falling back to the webhook notifier for everyone (spec AC2.3).
type ChatOAuthService interface {
	GetAccessToken(ctx context.Context) (string, error)
}
