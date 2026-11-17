package chat_interactive_service

import (
	"context"

	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/move_fields"
)

// ChatInteractiveService answers the Nudge commands. Every command maps to a
// feature of the web dashboard and calls the same services the web uses:
//
//	/n-dashboard  Dashboard: tickets being worked on + budget, overdue, mute, leave
//	/n-tiket      Papan Tiket: active-sprint tickets, open one to move it
//	/n-cuti       Cuti: list, add (form or typed dates), remove
//	/n-mute       Mute / unmute notifications
//	/n-help       Menu with a button per feature
//
// Every outcome is a normal chat reply, never an error.
type ChatInteractiveService interface {
	// RouteCommand is the text-only reply (operator DM poller).
	RouteCommand(ctx context.Context, senderEmail, messageText string) string

	// Command is the reply with cards for the Chat App; Text is its plain
	// rendering (notification preview, admin log, text-only senders).
	Command(ctx context.Context, senderEmail, messageText string) Reply

	// Action answers a card button (function name starting with ActionPrefix).
	// params holds the button parameters merged with the card's form values.
	Action(ctx context.Context, senderEmail, function string, params map[string]string) ActionReply

	// Geser moves one of the sender's own active-sprint tickets to "todo" or
	// "code-review" (reminder card buttons and the web's target endpoint).
	Geser(ctx context.Context, senderEmail, issueKey, targetSlug string) GeserResult
}

// ActionPrefix starts the function name of every button Command builds.
const ActionPrefix = "nudge."

// Reply is a command's answer.
type Reply struct {
	Text  string
	Cards []chat_app_dto.CardWithID
	// Title is the one line sent as message text above the cards — what
	// Google Chat shows in Home and notifications (a card-only message
	// shows "Sent an attachment" there).
	Title string
}

// ActionReply is a button's answer. Update replaces the card that was
// clicked; otherwise the reply is posted as a new message.
type ActionReply struct {
	Reply
	Update bool
}

// GeserResult is the outcome of Geser.
type GeserResult struct {
	OK         bool
	NotOwner   bool
	StatusName string
	Message    string
	// FieldsRequired: the move needs fields filled first (not moved);
	// Message then says which and links the web dialog.
	FieldsRequired *move_fields.Required
}
