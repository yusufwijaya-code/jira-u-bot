package chat_app_repository

import (
	"context"

	"nudge/modules/chat_app/chat_app_dto"
)

// Repository reads and writes the Chat App columns of ms_user (Tech Spec
// "Nudge sebagai Google Chat App"). Kept apart from RegistrationRepository
// so its many test fakes don't all have to grow these methods.
type Repository interface {
	// FindByEmail returns found=false (and a State carrying only Email)
	// for an email that isn't registered.
	FindByEmail(ctx context.Context, email string) (chat_app_dto.State, bool, error)
	// ListAll keys every registered user's state by lower-cased email.
	ListAll(ctx context.Context) (map[string]chat_app_dto.State, error)
	// SetSpace stores the user's DM space with the app; the activation date
	// only changes when the space does. An empty chatUserID keeps the old one.
	SetSpace(ctx context.Context, email, spaceID, chatUserID string) error
	// ClearBySpace / ClearByEmail forget the DM and reset the sender choice
	// to automatic (the app was removed).
	ClearBySpace(ctx context.Context, spaceID string) error
	ClearByEmail(ctx context.Context, email string) error
	// SetSender stores the sender choice; nil = automatic.
	SetSender(ctx context.Context, email string, sender *string) error
}
