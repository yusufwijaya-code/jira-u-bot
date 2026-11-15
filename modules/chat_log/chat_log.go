// Package chat_log wires the admin "Riwayat pesan" into the legacy
// operator-DM command poller and purges old entries.
package chat_log

import (
	"context"
	"log"
	"time"

	"nudge/modules/chat_interactive/chat_interactive_service"
	"nudge/modules/chat_log/chat_log_dto"
)

// LoggedCommands records each command a user sends to the operator DM
// (the legacy poller) before routing it.
type LoggedCommands struct {
	chat_interactive_service.ChatInteractiveService
	Log chat_log_dto.Recorder
}

func (l LoggedCommands) RouteCommand(ctx context.Context, senderEmail, messageText string) string {
	l.Log.Record(ctx, chat_log_dto.Entry{
		Email: senderEmail, Direction: chat_log_dto.DirectionIn, Kind: chat_log_dto.KindCommand,
		Channel: chat_log_dto.ChannelOperator, Status: chat_log_dto.StatusReceived, Body: messageText,
	})
	return l.ChatInteractiveService.RouteCommand(ctx, senderEmail, messageText)
}

// DMSender is the operator DM the legacy poller replies through.
type DMSender interface {
	SendPlainTextDM(ctx context.Context, email, text string) error
}

// LoggedReplies records the poller's replies sent through the operator DM.
type LoggedReplies struct {
	Inner DMSender
	Log   chat_log_dto.Recorder
}

func (l LoggedReplies) SendPlainTextDM(ctx context.Context, email, text string) error {
	err := l.Inner.SendPlainTextDM(ctx, email, text)
	e := chat_log_dto.Entry{
		Email: email, Direction: chat_log_dto.DirectionOut, Kind: chat_log_dto.KindReply,
		Channel: chat_log_dto.ChannelOperator, Status: chat_log_dto.StatusSent, Body: text,
	}
	if err != nil {
		e.Status, e.Error = chat_log_dto.StatusFailed, err.Error()
	}
	l.Log.Record(ctx, e)
	return err
}

// Purger deletes entries older than a cut-off.
type Purger interface {
	PurgeBefore(ctx context.Context, t time.Time) (int64, error)
}

// RunRetention deletes entries older than keep, now and then once a day,
// until ctx ends.
func RunRetention(ctx context.Context, p Purger, keep time.Duration) {
	purge := func() {
		n, err := p.PurgeBefore(ctx, time.Now().Add(-keep))
		if err != nil {
			log.Printf("chat_log: purge failed: %v", err)
			return
		}
		if n > 0 {
			log.Printf("chat_log: purged %d entries older than %s", n, keep)
		}
	}
	purge()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			purge()
		}
	}
}
