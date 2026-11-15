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
	"nudge/modules/chat_log/chat_log_dto"
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
		"💡 Ketik */n-help* buat lihat command\n\n" +
		"_Pesan & command di DM ini dicatat untuk keperluan admin Nudge._"
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
	Command(ctx context.Context, senderEmail, messageText string) chat_interactive_service.Reply
	Action(ctx context.Context, senderEmail, function string, params map[string]string) chat_interactive_service.ActionReply
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
	// Log records events and replies for the admin "Riwayat pesan" (nil = off).
	Log chat_log_dto.Recorder
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
	s.recordEvent(ctx, ev)
	resp := s.handle(ctx, ev)
	if text := responseText(resp); text != "" {
		s.record(ctx, chat_log_dto.Entry{
			Email: ev.User.Email, Direction: chat_log_dto.DirectionOut, Kind: chat_log_dto.KindReply,
			Channel: chat_log_dto.ChannelApp, Status: chat_log_dto.StatusSent, Body: text,
		})
	}
	return resp
}

func (s *service) record(ctx context.Context, e chat_log_dto.Entry) {
	if s.d.Log != nil && e.Email != "" {
		s.d.Log.Record(ctx, e)
	}
}

// recordEvent logs what the user did: a message or command, a button
// click, or adding/removing the app.
func (s *service) recordEvent(ctx context.Context, ev chat_app_dto.Event) {
	e := chat_log_dto.Entry{Email: ev.User.Email, Direction: chat_log_dto.DirectionIn, Channel: chat_log_dto.ChannelApp, Status: chat_log_dto.StatusReceived}
	switch ev.Type {
	case chat_app_dto.EventAddedToSpace:
		e.Kind, e.Body = chat_log_dto.KindAdded, "Menambahkan app Nudge"
	case chat_app_dto.EventRemovedFromSpace:
		e.Kind, e.Body = chat_log_dto.KindRemoved, "Menghapus app Nudge"
	case chat_app_dto.EventMessage:
		e.Kind = chat_log_dto.KindCommand
		if ev.Message != nil {
			e.Body = strings.TrimSpace(ev.Message.Text)
		}
		if e.Body == "" {
			e.Body = commandText(ev.Message)
		}
	case chat_app_dto.EventCardClicked:
		e.Kind, e.Body = chat_log_dto.KindClick, "Klik tombol"
		switch {
		case ev.Action == nil:
		case ev.Action.ActionMethodName == chat_app_card.GeserFunction:
			e.Body = fmt.Sprintf("Klik tombol: geser %s ke %s", ev.Action.Param("issueKey"), ev.Action.Param("target"))
		case ev.Action.Param("label") != "":
			e.Body = "Klik tombol: " + ev.Action.Param("label")
			if key := ev.Action.Param("key"); key != "" {
				e.Body += " (" + key + ")"
			}
		}
	default:
		return
	}
	s.record(ctx, e)
}

// responseText is the text a sync response shows (text or card status).
func responseText(r chat_app_dto.Response) string {
	if r.LogText != "" {
		return r.LogText
	}
	if r.Text != "" {
		return r.Text
	}
	if len(r.CardsV2) > 0 {
		return "(kartu diperbarui)"
	}
	return ""
}

func (s *service) handle(ctx context.Context, ev chat_app_dto.Event) chat_app_dto.Response {
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
	text := commandText(ev.Message)
	return replyResponse(s.runCommand(email, ev.Space.Name, func(ctx context.Context) chat_interactive_service.Reply {
		return s.d.Commands.Command(ctx, email, text)
	}), false)
}

// slashCommands maps the Command IDs registered in the Chat App
// configuration to the /n-* text chat_interactive understands. Old names
// typed as text still work (chat_interactive keeps them as aliases).
var slashCommands = map[string]string{
	"1": "/n-mytickets",
	"2": "/n-dashboard",
	"3": "/n-cuti",
	"4": "/n-mute",
	"5": "/n-help",
	"6": "/n-unmute",
}

// commandText is the /n-* text a message stands for.
func commandText(msg *chat_app_dto.Message) string {
	if msg == nil {
		return ""
	}
	if msg.SlashCommand != nil {
		name, ok := slashCommands[string(msg.SlashCommand.CommandID)]
		if !ok {
			return "/n-help"
		}
		if args := strings.TrimSpace(msg.ArgumentText); args != "" {
			return name + " " + args
		}
		return name
	}
	if arg := strings.TrimSpace(msg.ArgumentText); arg != "" {
		return arg
	}
	return strings.TrimSpace(msg.Text)
}

// replyResponse turns a command reply into the event response: a card
// message (or a card update) when the reply has cards, else text.
func replyResponse(r chat_interactive_service.Reply, update bool) chat_app_dto.Response {
	resp := chat_app_dto.Response{LogText: r.Text}
	if len(r.Cards) > 0 {
		resp.CardsV2, resp.FallbackText = r.Cards, r.Title
	} else {
		resp.Text = r.Text
	}
	if update {
		resp.ActionResponse = &chat_app_dto.ActionResponse{Type: chat_app_dto.ResponseUpdateMessage}
	}
	return resp
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

// lateReply posts the reply wait returns through the app once it is ready.
func (s *service) lateReply(email, space string, wait func() chat_interactive_service.Reply) {
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
		e := chat_log_dto.Entry{
			Email: email, Direction: chat_log_dto.DirectionOut, Kind: chat_log_dto.KindReply,
			Channel: chat_log_dto.ChannelApp, Status: chat_log_dto.StatusSent, Body: out.Text,
		}
		msg := chat_app_dto.OutgoingMessage{Text: out.Text}
		if len(out.Cards) > 0 {
			msg = chat_app_dto.OutgoingMessage{FallbackText: out.Title, CardsV2: out.Cards}
		}
		if err := s.d.App.CreateMessage(ctx, space, msg); err != nil {
			log.Printf("chat_app_service: late reply to %s failed: %v", email, err)
			e.Status, e.Error = chat_log_dto.StatusFailed, err.Error()
		}
		s.record(ctx, e)
	}()
}

// runCommand answers within CommandTimeout; slower commands reply
// "processing" now and post the result through the app later (AC2.6).
func (s *service) runCommand(email, space string, work func(ctx context.Context) chat_interactive_service.Reply) chat_interactive_service.Reply {
	done := goSafe("command of "+email, chat_interactive_service.Reply{Text: errorText}, work)
	select {
	case out := <-done:
		return out
	case <-time.After(s.d.CommandTimeout):
		s.lateReply(email, space, func() chat_interactive_service.Reply { return <-done })
		return chat_interactive_service.Reply{Text: processingText}
	}
}

// cardClicked handles the reminder card's move buttons (AC6.3, AC6.4). It
// acts only as the clicking user (ev.User.Email), never a card parameter.
// Like commands it answers within CommandTimeout: a slow Jira gets
// "processing" now and the outcome as a new message later.
func (s *service) cardClicked(ctx context.Context, ev chat_app_dto.Event) chat_app_dto.Response {
	if ev.Action == nil {
		return chat_app_dto.Response{}
	}
	if strings.HasPrefix(ev.Action.ActionMethodName, chat_interactive_service.ActionPrefix) {
		return s.menuClicked(ctx, ev)
	}
	if ev.Action.ActionMethodName != chat_app_card.GeserFunction {
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
		s.lateReply(email, ev.Space.Name, func() chat_interactive_service.Reply {
			res := <-done
			if res.NotOwner {
				return chat_interactive_service.Reply{Text: res.Message}
			}
			return chat_interactive_service.Reply{Text: "*" + issueKey + "* — " + geserStatus(ev, res)}
		})
		return chat_app_dto.Response{ActionResponse: &chat_app_dto.ActionResponse{Type: chat_app_dto.ResponseNewMessage}, Text: processingText}
	}
}

// menuClicked answers the buttons of command cards: the reply replaces the
// clicked card (ActionReply.Update) or comes as a new message. A slow one
// answers "processing" now and posts the result later.
func (s *service) menuClicked(ctx context.Context, ev chat_app_dto.Event) chat_app_dto.Response {
	if reply, ok := s.checkUser(ctx, ev.User.Email); !ok {
		return s.private(ev, reply)
	}
	email, fn := ev.User.Email, ev.Action.ActionMethodName
	params := ev.FormValues()
	for _, p := range ev.Action.Parameters {
		params[p.Key] = p.Value
	}
	done := goSafe("action "+strconv.Quote(fn)+" of "+email,
		chat_interactive_service.ActionReply{Reply: chat_interactive_service.Reply{Text: errorText}},
		func(ctx context.Context) chat_interactive_service.ActionReply {
			return s.d.Commands.Action(ctx, email, fn, params)
		})
	select {
	case res := <-done:
		update := res.Update && ev.Message != nil && len(ev.Message.CardsV2) > 0
		resp := replyResponse(res.Reply, update)
		if !update {
			resp.ActionResponse = &chat_app_dto.ActionResponse{Type: chat_app_dto.ResponseNewMessage}
		}
		return resp
	case <-time.After(s.d.CommandTimeout):
		s.lateReply(email, ev.Space.Name, func() chat_interactive_service.Reply { return (<-done).Reply })
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
