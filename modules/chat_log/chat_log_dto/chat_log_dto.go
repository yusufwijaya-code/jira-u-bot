// Package chat_log_dto holds the "Riwayat pesan" types: one Entry per
// message Nudge sends and per Chat event or command a user sends it.
package chat_log_dto

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DirectionIn  = "in"
	DirectionOut = "out"

	KindReminder = "reminder" // reminder or status-change notice
	KindMessage  = "message"  // any other notice (broadcast, settings change…)
	KindReply    = "reply"    // answer to a command or click
	KindCommand  = "command"  // text or slash command from the user
	KindClick    = "click"    // card button click
	KindAdded    = "added"    // app added to the DM
	KindRemoved  = "removed"  // app removed from the DM

	ChannelApp      = "app"
	ChannelOperator = "operator"
	ChannelFallback = "fallback"

	StatusReceived = "received"
	StatusSent     = "sent"
	StatusFailed   = "failed"
	StatusSkipped  = "skipped"

	MaxBodyRunes  = 4000
	MaxErrorRunes = 500
)

type Entry struct {
	ID          int64     `json:"id"`
	Email       string    `json:"email"`
	Direction   string    `json:"direction"`
	Kind        string    `json:"kind"`
	Channel     string    `json:"channel"`
	Status      string    `json:"status"`
	Body        string    `json:"body"`
	Error       string    `json:"error,omitempty"`
	CreatedDate time.Time `json:"createdDate"`
}

// Recorder stores entries. Implementations never fail the caller: a
// recording problem is only logged.
type Recorder interface {
	Record(ctx context.Context, e Entry)
}

// Filter narrows the list; zero values mean "no filter". Results are newest
// first; BeforeID pages backwards (entries with a smaller ID).
type Filter struct {
	Email    string
	From     time.Time
	To       time.Time
	BeforeID int64
	// AfterID keeps only entries newer than this ID (live polling).
	AfterID int64
	Limit   int
	// Kinds keeps only these kinds; empty = all.
	Kinds []string
}

// Conversation is one user's thread with Nudge: its size and last entry.
type Conversation struct {
	Email         string    `json:"email"`
	Total         int       `json:"total"`
	LastDirection string    `json:"lastDirection"`
	LastKind      string    `json:"lastKind"`
	LastStatus    string    `json:"lastStatus"`
	LastBody      string    `json:"lastBody"`
	LastDate      time.Time `json:"lastDate"`
}

type Page struct {
	Items []Entry `json:"items"`
	// NextBeforeID is the BeforeID of the next (older) page; 0 = no more.
	NextBeforeID int64 `json:"nextBeforeID"`
}

// Truncate cuts s to at most n runes, marking the cut.
func Truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}
