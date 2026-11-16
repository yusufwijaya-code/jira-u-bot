// Package chat_app_dto holds the Google Chat App shapes Nudge uses: the
// per-user activation state (ms_user columns), the HTTP event Google posts
// to /api/chat/events and the synchronous reply (Tech Spec "Nudge sebagai
// Google Chat App").
package chat_app_dto

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Message senders a user can choose in Pengaturan.
const (
	SenderNudge    = "nudge"
	SenderOperator = "operator"
)

// ValidSender reports whether s is one of the two sender values.
func ValidSender(s string) bool { return s == SenderNudge || s == SenderOperator }

// State is a user's Chat App activation and sender choice. Sender nil =
// automatic (nudge when activated, operator otherwise).
type State struct {
	Email         string
	SpaceID       string
	ChatUserID    string
	ActivatedDate *time.Time
	Sender        *string
	// GroupAddedDate: when the email joined the group that makes the app
	// visible (nil = not yet); GroupError: the last failure doing so.
	GroupAddedDate *time.Time
	GroupError     string
}

// Activated reports whether the user has a DM with the Nudge app.
func (s State) Activated() bool { return s.SpaceID != "" }

// Event types Nudge handles.
const (
	EventAddedToSpace     = "ADDED_TO_SPACE"
	EventRemovedFromSpace = "REMOVED_FROM_SPACE"
	EventMessage          = "MESSAGE"
	EventCardClicked      = "CARD_CLICKED"
)

// Event is the interaction event Google Chat posts to the HTTP endpoint.
type Event struct {
	Type    string   `json:"type"`
	Space   Space    `json:"space"`
	User    ChatUser `json:"user"`
	Message *Message `json:"message,omitempty"`
	Action  *Action  `json:"action,omitempty"`
	Common  *Common  `json:"common,omitempty"`
}

// Common carries the values of a card's form fields on CARD_CLICKED.
type Common struct {
	FormInputs map[string]FormInput `json:"formInputs,omitempty"`
}

type FormInput struct {
	StringInputs *StringInputs `json:"stringInputs,omitempty"`
	DateInput    *DateInput    `json:"dateInput,omitempty"`
}

type StringInputs struct {
	Value []string `json:"value"`
}

type DateInput struct {
	MsSinceEpoch FlexString `json:"msSinceEpoch"`
}

// FormValues flattens the form inputs: text fields to their first value,
// date fields to YYYY-MM-DD (a DATE_ONLY value is UTC midnight of the day).
func (e Event) FormValues() map[string]string {
	out := map[string]string{}
	if e.Common == nil {
		return out
	}
	for name, in := range e.Common.FormInputs {
		switch {
		case in.DateInput != nil:
			if ms, err := strconv.ParseInt(string(in.DateInput.MsSinceEpoch), 10, 64); err == nil && ms > 0 {
				out[name] = time.UnixMilli(ms).UTC().Format("2006-01-02")
			}
		case in.StringInputs != nil && len(in.StringInputs.Value) > 0:
			out[name] = in.StringInputs.Value[0]
		}
	}
	return out
}

type Space struct {
	Name      string `json:"name"`
	Type      string `json:"type,omitempty"`
	SpaceType string `json:"spaceType,omitempty"`
}

// IsDM covers both the legacy "type" and the newer "spaceType" field.
func (s Space) IsDM() bool { return s.Type == "DM" || s.SpaceType == "DIRECT_MESSAGE" }

type ChatUser struct {
	Name        string `json:"name"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Type        string `json:"type,omitempty"`
}

type Message struct {
	Name         string        `json:"name,omitempty"`
	Text         string        `json:"text,omitempty"`
	ArgumentText string        `json:"argumentText,omitempty"`
	SlashCommand *SlashCommand `json:"slashCommand,omitempty"`
	CardsV2      []CardWithID  `json:"cardsV2,omitempty"`
}

type SlashCommand struct {
	CommandID FlexString `json:"commandId"`
}

// FlexString accepts a JSON string or number (Google sends int64 ids as
// strings, but tolerating a bare number costs nothing).
type FlexString string

func (f *FlexString) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = FlexString(s)
		return nil
	}
	*f = FlexString(strings.TrimSpace(string(b)))
	return nil
}

type Action struct {
	ActionMethodName string            `json:"actionMethodName"`
	Parameters       []ActionParameter `json:"parameters,omitempty"`
}

// Param returns the value of the action parameter key ("" when absent).
func (a *Action) Param(key string) string {
	if a == nil {
		return ""
	}
	for _, p := range a.Parameters {
		if p.Key == key {
			return p.Value
		}
	}
	return ""
}

type ActionParameter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Response is the synchronous reply to an event; the zero value marshals
// to {} (no reply).
type Response struct {
	Text string `json:"text,omitempty"`
	// FallbackText is the notification preview of a card-only message.
	FallbackText         string          `json:"fallbackText,omitempty"`
	CardsV2              []CardWithID    `json:"cardsV2,omitempty"`
	ActionResponse       *ActionResponse `json:"actionResponse,omitempty"`
	PrivateMessageViewer *ChatUser       `json:"privateMessageViewer,omitempty"`
	// LogText is what the admin "Pesan" log shows for this reply (a card's
	// text rendering); never sent to Google.
	LogText string `json:"-"`
}

type ActionResponse struct {
	Type string `json:"type"`
}

const (
	ResponseNewMessage    = "NEW_MESSAGE"
	ResponseUpdateMessage = "UPDATE_MESSAGE"
)

// OutgoingMessage is a message Nudge creates through the Chat API.
type OutgoingMessage struct {
	Text         string       `json:"text,omitempty"`
	FallbackText string       `json:"fallbackText,omitempty"`
	CardsV2      []CardWithID `json:"cardsV2,omitempty"`
}
