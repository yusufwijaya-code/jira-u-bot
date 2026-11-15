// Package chat_app_card renders reminders and status notices as Google
// Chat cards with action buttons (Tech Spec "Nudge sebagai Google Chat
// App", US-6). Only the app path uses cards; the operator DM keeps text.
package chat_app_card

import (
	"html"
	"regexp"
	"strings"

	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/google_chat_notifier/google_chat_notifier_dto"
	"nudge/modules/google_chat_notifier/google_chat_notifier_service"
)

// GeserFunction is the action name of the move buttons (CARD_CLICKED).
const GeserFunction = "geser"

var boldPattern = regexp.MustCompile(`\*([^*\n]+)\*`)

// ToCardHTML turns Chat markup (*bold*, \n) into the HTML subset card
// text paragraphs render, escaping everything else.
func ToCardHTML(chatText string) string {
	s := html.EscapeString(chatText)
	s = boldPattern.ReplaceAllString(s, "<b>$1</b>")
	return strings.ReplaceAll(s, "\n", "<br>")
}

type move struct {
	label, target string
}

// Build is a chat_delivery.CardBuilder: the short text shown in
// notifications plus one card.
func Build(msg google_chat_notifier_dto.ReminderMessage) (string, []chat_app_dto.CardWithID) {
	var short, body string
	var moves []move
	if msg.Text != "" {
		// Status change notice (AC6.2): Jira link only.
		short, body, _ = strings.Cut(msg.Text, "\n")
		if body == "" {
			body = short
		}
	} else {
		short = google_chat_notifier_service.MessageHeader(msg)
		body = google_chat_notifier_service.BuildMessageText(msg)
		// Same targets /n-geser supports; a ticket in review only goes back.
		if !msg.InReview {
			moves = append(moves, move{"Geser ke Code Review", "code-review"})
		}
		moves = append(moves, move{"Geser ke To Do", "todo"})
	}

	var bs []chat_app_dto.Button
	for _, m := range moves {
		bs = append(bs, chat_app_dto.Button{Text: m.label, OnClick: chat_app_dto.OnClick{Action: &chat_app_dto.OnClickAction{
			Function: GeserFunction,
			Parameters: []chat_app_dto.ActionParameter{
				{Key: "issueKey", Value: msg.IssueKey},
				{Key: "target", Value: m.target},
			},
		}}})
	}
	if msg.IssueURL != "" {
		bs = append(bs, chat_app_dto.Button{Text: "Buka di Jira", OnClick: chat_app_dto.OnClick{OpenLink: &chat_app_dto.OpenLink{URL: msg.IssueURL}}})
	}

	widgets := []chat_app_dto.Widget{{TextParagraph: &chat_app_dto.TextParagraph{Text: ToCardHTML(body)}}}
	if len(bs) > 0 {
		widgets = append(widgets, chat_app_dto.Widget{ButtonList: &chat_app_dto.ButtonList{Buttons: bs}})
	}
	card := chat_app_dto.Card{
		Header:   &chat_app_dto.CardHeader{Title: msg.IssueKey, Subtitle: msg.IssueSummary},
		Sections: []chat_app_dto.Section{{Widgets: widgets}},
	}
	return short, []chat_app_dto.CardWithID{{CardID: "nudge-" + msg.IssueKey, Card: card}}
}

// StripActions returns a copy of card without action buttons (links stay)
// and with status appended as its last section (AC6.3, AC6.4).
func StripActions(card chat_app_dto.Card, status string) chat_app_dto.Card {
	out := chat_app_dto.Card{Header: card.Header}
	for _, s := range card.Sections {
		var ws []chat_app_dto.Widget
		for _, w := range s.Widgets {
			if w.ButtonList == nil {
				ws = append(ws, w)
				continue
			}
			var keep []chat_app_dto.Button
			for _, b := range w.ButtonList.Buttons {
				if b.OnClick.Action == nil {
					keep = append(keep, b)
				}
			}
			if len(keep) > 0 {
				ws = append(ws, chat_app_dto.Widget{ButtonList: &chat_app_dto.ButtonList{Buttons: keep}})
			}
		}
		out.Sections = append(out.Sections, chat_app_dto.Section{Widgets: ws})
	}
	out.Sections = append(out.Sections, chat_app_dto.Section{Widgets: []chat_app_dto.Widget{{TextParagraph: &chat_app_dto.TextParagraph{Text: status}}}})
	return out
}
