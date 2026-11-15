// Package chat_app_service answers the events Google Chat posts to the
// Nudge Chat App (Tech Spec "Nudge sebagai Google Chat App", US-2).
package chat_app_service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"nudge/modules/chat_app/chat_app_card"
	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/chat_interactive/chat_interactive_service"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/user_setting/user_setting_dto"
)

const (
	defaultCommandTimeout = 25 * time.Second
	asyncCommandTimeout   = 2 * time.Minute

	processingText    = "⏳ Sedang diproses…"
	errorText         = "Terjadi kesalahan, coba lagi."
	notRegisteredText = "Kamu belum terdaftar di Nudge. Login dulu ke https://nudge.labcup.tech lalu kirim pesan apa saja di sini."
	inactiveText      = "Akun Nudge kamu sedang dinonaktifkan admin."
	welcomeText       = "✅ App Nudge aktif! Notifikasi Nudge sekarang bisa datang langsung dari sini.\n\n" +
		"Atur pengirim pesan di *Pengaturan*: https://nudge.labcup.tech/dashboard/settings\n\n" +
		"💡 Ketik */n-help* buat lihat command"
)

type UserFinder interface {
	FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error)
}

type StateStore interface {
	FindByEmail(ctx context.Context, email string) (chat_app_dto.State, bool, error)
	SetSpace(ctx context.Context, email, spaceID, chatUserID string) error
	ClearBySpace(ctx context.Context, spaceID string) error
}

type CommandRouter interface {
	RouteCommand(ctx context.Context, senderEmail, messageText string) string
	Geser(ctx context.Context, senderEmail, issueKey, targetSlug string) chat_interactive_service.GeserResult
}

type AppSender interface {
	CreateMessage(ctx context.Context, space string, msg chat_app_dto.OutgoingMessage) error
}

type Deps struct {
	Users    UserFinder
	States   StateStore
	Commands CommandRouter
	App      AppSender
	// CommandTimeout bounds the synchronous reply (Google waits ~30s); 0 = 25s.
	CommandTimeout time.Duration
}

type Service interface {
	Handle(ctx context.Context, ev chat_app_dto.Event) chat_app_dto.Response
}

type service struct {
	d Deps
}

func New(d Deps) Service {
	if d.CommandTimeout <= 0 {
		d.CommandTimeout = defaultCommandTimeout
	}
	return &service{d: d}
}

func (s *service) Handle(ctx context.Context, ev chat_app_dto.Event) chat_app_dto.Response {
	if !ev.Space.IsDM() {
		return chat_app_dto.Response{}
	}
	switch ev.Type {
	case chat_app_dto.EventAddedToSpace:
		return s.added(ctx, ev)
	case chat_app_dto.EventRemovedFromSpace:
		if err := s.d.States.ClearBySpace(ctx, ev.Space.Name); err != nil {
			log.Printf("chat_app_service: clearing %s failed: %v", ev.Space.Name, err)
		}
		return chat_app_dto.Response{}
	case chat_app_dto.EventMessage:
		return s.message(ctx, ev)
	case chat_app_dto.EventCardClicked:
		return s.cardClicked(ctx, ev)
	}
	return chat_app_dto.Response{}
}

// checkUser returns the reply for a sender that may not use Nudge.
func (s *service) checkUser(ctx context.Context, email string) (string, bool) {
	u, found, err := s.d.Users.FindUserByEmail(ctx, email)
	switch {
	case err != nil:
		log.Printf("chat_app_service: FindUserByEmail(%s) failed: %v", email, err)
		return errorText, false
	case !found:
		return notRegisteredText, false
	case !u.FlagActive:
		return inactiveText, false
	}
	return "", true
}

func (s *service) added(ctx context.Context, ev chat_app_dto.Event) chat_app_dto.Response {
	if reply, ok := s.checkUser(ctx, ev.User.Email); !ok {
		return chat_app_dto.Response{Text: reply}
	}
	if err := s.d.States.SetSpace(ctx, ev.User.Email, ev.Space.Name, ev.User.Name); err != nil {
		log.Printf("chat_app_service: storing the DM of %s failed: %v", ev.User.Email, err)
		return chat_app_dto.Response{Text: errorText}
	}
	return chat_app_dto.Response{Text: welcomeText}
}

func (s *service) message(ctx context.Context, ev chat_app_dto.Event) chat_app_dto.Response {
	email := ev.User.Email
	if reply, ok := s.checkUser(ctx, email); !ok {
		return chat_app_dto.Response{Text: reply}
	}
	// The install event may have been missed: remember the DM on any message.
	if state, _, err := s.d.States.FindByEmail(ctx, email); err == nil && state.SpaceID != ev.Space.Name {
		if err := s.d.States.SetSpace(ctx, email, ev.Space.Name, ev.User.Name); err != nil {
			log.Printf("chat_app_service: storing the DM of %s failed: %v", email, err)
		}
	}
	text, usage := commandText(ev.Message)
	if usage != "" {
		return chat_app_dto.Response{Text: usage}
	}
	return chat_app_dto.Response{Text: s.runCommand(email, ev.Space.Name, text)}
}

const geserUsage = "Format: */n-geser <issueKey> <todo|code-review>*, contoh */n-geser CORE-123 code-review*"

// slashCommands maps the Command IDs registered in the Chat App
// configuration to the /n-* text chat_interactive understands (AC5.1).
var slashCommands = map[string]string{
	"1": "/n-mytickets",
	"2": "/n-budget",
	"3": "/n-overdue",
	"4": "/n-team",
	"5": "/n-help",
	"6": "/n-geser",
}

// commandText is the /n-* text a message stands for; a non-empty second
// value is a usage hint to reply with instead.
func commandText(msg *chat_app_dto.Message) (string, string) {
	if msg == nil {
		return "", ""
	}
	if msg.SlashCommand != nil {
		name, ok := slashCommands[string(msg.SlashCommand.CommandID)]
		if !ok {
			return "/n-help", ""
		}
		args := strings.TrimSpace(msg.ArgumentText)
		if name == "/n-geser" {
			fields := strings.Fields(args)
			if len(fields) != 2 {
				return "", geserUsage
			}
			return fmt.Sprintf("/n-geser<%s>:%s", strings.ToUpper(fields[0]), strings.ToLower(fields[1])), ""
		}
		if args != "" {
			return name + " " + args, ""
		}
		return name, ""
	}
	if arg := strings.TrimSpace(msg.ArgumentText); arg != "" {
		return arg, ""
	}
	return strings.TrimSpace(msg.Text), ""
}

// runCommand answers within CommandTimeout; slower commands reply
// "processing" now and post the result through the app later.
func (s *service) runCommand(email, space, text string) string {
	done := make(chan string, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), asyncCommandTimeout)
		defer cancel()
		done <- s.d.Commands.RouteCommand(ctx, email, text)
	}()
	select {
	case out := <-done:
		return out
	case <-time.After(s.d.CommandTimeout):
		go func() {
			out := <-done
			if s.d.App == nil {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := s.d.App.CreateMessage(ctx, space, chat_app_dto.OutgoingMessage{Text: out}); err != nil {
				log.Printf("chat_app_service: late reply to %s failed: %v", email, err)
			}
		}()
		return processingText
	}
}

// cardClicked handles the reminder card's move buttons (AC6.3, AC6.4). It
// acts only as the clicking user (ev.User.Email), never a card parameter.
func (s *service) cardClicked(ctx context.Context, ev chat_app_dto.Event) chat_app_dto.Response {
	if ev.Action == nil || ev.Action.ActionMethodName != chat_app_card.GeserFunction {
		return chat_app_dto.Response{}
	}
	if reply, ok := s.checkUser(ctx, ev.User.Email); !ok {
		return s.private(ev, reply)
	}
	res := s.d.Commands.Geser(ctx, ev.User.Email, ev.Action.Param("issueKey"), ev.Action.Param("target"))
	if res.NotOwner {
		return s.private(ev, res.Message)
	}
	var status string
	if res.OK {
		now := time.Now().In(user_setting_dto.Jakarta)
		status = fmt.Sprintf("✅ Digeser ke %s oleh %s · %s WIB", res.StatusName, ev.User.DisplayName, now.Format("15:04"))
	} else {
		status = "⚠️ Tidak bisa digeser: " + res.Message
	}
	if ev.Message == nil || len(ev.Message.CardsV2) == 0 {
		return chat_app_dto.Response{ActionResponse: &chat_app_dto.ActionResponse{Type: chat_app_dto.ResponseNewMessage}, Text: status}
	}
	cards := make([]chat_app_dto.CardWithID, 0, len(ev.Message.CardsV2))
	for _, c := range ev.Message.CardsV2 {
		cards = append(cards, chat_app_dto.CardWithID{CardID: c.CardID, Card: chat_app_card.StripActions(c.Card, chat_app_card.ToCardHTML(status))})
	}
	return chat_app_dto.Response{ActionResponse: &chat_app_dto.ActionResponse{Type: chat_app_dto.ResponseUpdateMessage}, CardsV2: cards}
}

// private answers only the clicking user, leaving the card as it is.
func (s *service) private(ev chat_app_dto.Event, text string) chat_app_dto.Response {
	viewer := ev.User
	return chat_app_dto.Response{
		ActionResponse:       &chat_app_dto.ActionResponse{Type: chat_app_dto.ResponseNewMessage},
		Text:                 text,
		PrivateMessageViewer: &viewer,
	}
}
