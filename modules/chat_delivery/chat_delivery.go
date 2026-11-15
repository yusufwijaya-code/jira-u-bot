// Package chat_delivery routes every outgoing Nudge message to the sender
// the recipient chose (Tech Spec "Nudge sebagai Google Chat App", AC1.x):
// the Nudge Chat App or the operator account's DM. Any app failure falls
// back once to the operator DM — except for the operator account itself,
// which can't DM itself.
package chat_delivery

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"nudge/modules/chat_app/chat_app_client"
	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/chat_log/chat_log_dto"
	"nudge/modules/google_chat_notifier/google_chat_notifier_dto"
	"nudge/modules/google_chat_notifier/google_chat_notifier_service"
)

// ErrRecipientNotActivated means no sender can reach the recipient (e.g.
// the operator account before activating Nudge). Callers record it as
// skipped, not failed.
var ErrRecipientNotActivated = errors.New("chat_delivery: recipient has no usable chat sender")

const inviteFooter = "\n\n💡 Pesan ini bisa datang langsung dari app Nudge — aktifkan di https://nudge.labcup.tech"

type StateStore interface {
	FindByEmail(ctx context.Context, email string) (chat_app_dto.State, bool, error)
	ClearByEmail(ctx context.Context, email string) error
}

type AppSender interface {
	CreateMessage(ctx context.Context, space string, msg chat_app_dto.OutgoingMessage) error
}

type LegacySender interface {
	SendPlainTextDM(ctx context.Context, email, text string) error
}

// CardBuilder turns a reminder into the app message (fallback text + cards).
type CardBuilder func(msg google_chat_notifier_dto.ReminderMessage) (string, []chat_app_dto.CardWithID)

type Deps struct {
	Store StateStore
	// App is nil when the Chat App is not configured.
	App AppSender
	// Legacy is nil when the operator OAuth is not configured.
	Legacy        LegacySender
	OperatorEmail string
	OperatorName  string
	// Cards is nil until phase 3: the app then gets the plain text.
	Cards CardBuilder
	// Log records every send for the admin "Riwayat pesan" (nil = off).
	Log chat_log_dto.Recorder
}

type Router struct {
	d Deps
}

func New(d Deps) *Router {
	if d.OperatorName == "" {
		d.OperatorName = "operator Nudge"
	}
	return &Router{d: d}
}

// IsOperator reports whether email is the operator (OAuth / bot) account.
func IsOperator(email, operatorEmail string) bool {
	return operatorEmail != "" && strings.EqualFold(strings.TrimSpace(email), strings.TrimSpace(operatorEmail))
}

// EffectiveSender is the sender a message to this user goes through.
func EffectiveSender(s chat_app_dto.State, isOperator, appEnabled bool) string {
	switch {
	case !appEnabled:
		return chat_app_dto.SenderOperator
	case isOperator:
		return chat_app_dto.SenderNudge
	case s.Sender != nil && *s.Sender == chat_app_dto.SenderOperator:
		return chat_app_dto.SenderOperator
	case s.Sender != nil:
		return chat_app_dto.SenderNudge
	case s.Activated():
		return chat_app_dto.SenderNudge
	}
	return chat_app_dto.SenderOperator
}

func (r *Router) SendReminder(ctx context.Context, msg google_chat_notifier_dto.ReminderMessage) error {
	text := google_chat_notifier_service.BuildMessageText(msg)
	appMsg := chat_app_dto.OutgoingMessage{Text: text}
	if r.d.Cards != nil {
		appMsg.Text, appMsg.CardsV2 = r.d.Cards(msg)
	}
	return r.logged(ctx, chat_log_dto.KindReminder, msg.AssigneeEmail, text, appMsg)
}

func (r *Router) SendPlainTextDM(ctx context.Context, email, text string) error {
	return r.logged(ctx, chat_log_dto.KindMessage, email, text, chat_app_dto.OutgoingMessage{Text: text})
}

// logged delivers and records the outcome for the admin "Riwayat pesan".
func (r *Router) logged(ctx context.Context, kind, email, text string, appMsg chat_app_dto.OutgoingMessage) error {
	channel, err := r.deliver(ctx, email, text, appMsg)
	if r.d.Log != nil {
		e := chat_log_dto.Entry{
			Email: email, Direction: chat_log_dto.DirectionOut, Kind: kind,
			Channel: channel, Status: chat_log_dto.StatusSent, Body: text,
		}
		switch {
		case errors.Is(err, ErrRecipientNotActivated):
			e.Status = chat_log_dto.StatusSkipped
		case err != nil:
			e.Status, e.Error = chat_log_dto.StatusFailed, err.Error()
		}
		r.d.Log.Record(ctx, e)
	}
	return err
}

// deliver sends the message and reports the channel it went (or tried to
// go) through: app, operator or fallback.
func (r *Router) deliver(ctx context.Context, email, text string, appMsg chat_app_dto.OutgoingMessage) (string, error) {
	isOp := IsOperator(email, r.d.OperatorEmail)
	state, _, err := r.d.Store.FindByEmail(ctx, email)
	if err != nil {
		return "", fmt.Errorf("chat_delivery: load chat state of %s: %w", email, err)
	}
	appEnabled := r.d.App != nil

	if EffectiveSender(state, isOp, appEnabled) == chat_app_dto.SenderOperator {
		if isOp || r.d.Legacy == nil {
			return "", ErrRecipientNotActivated
		}
		if appEnabled && state.Sender == nil && !state.Activated() {
			text += inviteFooter
		}
		return chat_log_dto.ChannelOperator, r.d.Legacy.SendPlainTextDM(ctx, email, text)
	}

	appErr := ErrRecipientNotActivated
	if state.Activated() {
		err := r.d.App.CreateMessage(ctx, state.SpaceID, appMsg)
		if err == nil {
			return chat_log_dto.ChannelApp, nil
		}
		appErr = err
		if chat_app_client.IsGone(err) {
			if clearErr := r.d.Store.ClearByEmail(ctx, email); clearErr != nil {
				log.Printf("chat_delivery: forgetting the Chat App DM of %s failed: %v", email, clearErr)
			}
		}
	}
	if isOp || r.d.Legacy == nil {
		channel := ""
		if state.Activated() {
			channel = chat_log_dto.ChannelApp
		}
		return channel, appErr
	}

	log.Printf("chat_delivery: app send to %s failed, falling back to the operator DM: %v", email, appErr)
	footer := fmt.Sprintf("\n\n_(dikirim lewat akun %s karena app Nudge sedang gangguan)_", r.d.OperatorName)
	if err := r.d.Legacy.SendPlainTextDM(ctx, email, text+footer); err != nil {
		if errors.Is(appErr, ErrRecipientNotActivated) {
			return chat_log_dto.ChannelFallback, err
		}
		return chat_log_dto.ChannelFallback, errors.Join(appErr, err)
	}
	return chat_log_dto.ChannelFallback, nil
}
