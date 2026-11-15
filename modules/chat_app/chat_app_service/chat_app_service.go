// Package chat_app_service answers the events Google Chat posts to the
// Nudge Chat App (Tech Spec "Nudge sebagai Google Chat App", US-2).
package chat_app_service

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"strconv"
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
	lateReplyTimeout      = 15 * time.Second

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

// goSafe runs work in its own goroutine with a detached context (the
// request's ends when Google gets its reply). A panic is logged and turned
// into fallback, so it can't crash the process (AC2.7).
func goSafe[T any](what string, fallback T, work func(ctx context.Context) T) <-chan T {
	done := make(chan T, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), asyncCommandTimeout)
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("chat_app_service: %s panicked: %v\n%s", what, r, debug.Stack())
				done <- fallback
			}
		}()
		done <- work(ctx)
	}()
	return done
}

// lateReply posts the text wait returns through the app once it is ready.
func (s *service) lateReply(email, space string, wait func() string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("chat_app_service: late reply to %s panicked: %v\n%s", email, r, debug.Stack())
			}
		}()
		out := wait()
		if s.d.App == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), lateReplyTimeout)
		defer cancel()
		if err := s.d.App.CreateMessage(ctx, space, chat_app_dto.OutgoingMessage{Text: out}); err != nil {
			log.Printf("chat_app_service: late reply to %s failed: %v", email, err)
		}
	}()
}

// runCommand answers within CommandTimeout; slower commands reply
// "processing" now and post the result through the app later (AC2.6).
func (s *service) runCommand(email, space, text string) string {
	done := goSafe("command "+strconv.Quote(text)+" of "+email, errorText, func(ctx context.Context) string {
		return s.d.Commands.RouteCommand(ctx, email, text)
	})
	select {
	case out := <-done:
		return out
	case <-time.After(s.d.CommandTimeout):
		s.lateReply(email, space, func() string { return <-done })
		return processingText
	}
}

// cardClicked handles the reminder card's move buttons (AC6.3, AC6.4). It
// acts only as the clicking user (ev.User.Email), never a card parameter.
// Like commands it answers within CommandTimeout: a slow Jira gets
// "processing" now and the outcome as a new message later.
func (s *service) cardClicked(ctx context.Context, ev chat_app_dto.Event) chat_app_dto.Response {
	if ev.Action == nil || ev.Action.ActionMethodName != chat_app_card.GeserFunction {
		return chat_app_dto.Response{}
	}
	if reply, ok := s.checkUser(ctx, ev.User.Email); !ok {
		return s.private(ev, reply)
	}
	email, issueKey, target := ev.User.Email, ev.Action.Param("issueKey"), ev.Action.Param("target")
	done := goSafe("geser "+strconv.Quote(issueKey)+" by "+email, chat_interactive_service.GeserResult{Message: errorText},
		func(ctx context.Context) chat_interactive_service.GeserResult {
			return s.d.Commands.Geser(ctx, email, issueKey, target)
		})
	select {
	case res := <-done:
		return s.geserResponse(ev, res)
	case <-time.After(s.d.CommandTimeout):
		s.lateReply(email, ev.Space.Name, func() string {
			res := <-done
			if res.NotOwner {
				return res.Message
			}
			return "*" + issueKey + "* — " + geserStatus(ev, res)
		})
		return chat_app_dto.Response{ActionResponse: &chat_app_dto.ActionResponse{Type: chat_app_dto.ResponseNewMessage}, Text: processingText}
	}
}

// geserStatus is the status line of a move (AC6.3, AC6.4).
func geserStatus(ev chat_app_dto.Event, res chat_interactive_service.GeserResult) string {
	if res.OK {
		now := time.Now().In(user_setting_dto.Jakarta)
		return fmt.Sprintf("✅ Digeser ke %s oleh %s · %s WIB", res.StatusName, ev.User.DisplayName, now.Format("15:04"))
	}
	return "⚠️ Tidak bisa digeser: " + res.Message
}

func (s *service) geserResponse(ev chat_app_dto.Event, res chat_interactive_service.GeserResult) chat_app_dto.Response {
	if res.NotOwner {
		return s.private(ev, res.Message)
	}
	status := geserStatus(ev, res)
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
