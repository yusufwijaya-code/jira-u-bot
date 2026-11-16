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

// GeserFunction is the action name of the old move buttons (CARD_CLICKED);
// cards already in chats still send it.
const GeserFunction = "geser"

// Buttons answered by chat_interactive (its ActionPrefix "nudge." + name):
// open one ticket with a button per Jira workflow status, or run a command.
const (
	ticketFunction  = "nudge.ticket"
	commandFunction = "nudge.cmd"
)

// helpHintPattern matches the "💡 Ketik /n-help ..." line of a message;
// cards show a menu button instead.
var helpHintPattern = regexp.MustCompile(`(?m)^.*Ketik \*?/n-help\*?.*$\n?`)

// MenuButton opens the Nudge menu (/n-help).
func MenuButton() chat_app_dto.Button {
	return actionButton("📋 Buka menu Nudge", commandFunction, "cmd", "/n-help")
}

func geserButton(issueKey string) chat_app_dto.Button {
	return actionButton("🔀 Geser status", ticketFunction, "key", issueKey)
}

func actionButton(label, function string, kv ...string) chat_app_dto.Button {
	params := []chat_app_dto.ActionParameter{{Key: "label", Value: label}}
	for i := 0; i+1 < len(kv); i += 2 {
		params = append(params, chat_app_dto.ActionParameter{Key: kv[i], Value: kv[i+1]})
	}
	return chat_app_dto.Button{Text: label, OnClick: chat_app_dto.OnClick{Action: &chat_app_dto.OnClickAction{Function: function, Parameters: params}}}
}

// WithoutHelpHint drops the "Ketik /n-help" line (and trailing blank lines).
func WithoutHelpHint(text string) string {
	s := helpHintPattern.ReplaceAllString(text, "")
	return strings.TrimRight(blankLines.ReplaceAllString(s, "\n\n"), "\n ")
}

// TextCard turns a plain notice into a card when it advertises /n-help: the
// hint becomes a menu button. ok is false for any other text (sent as is).
func TextCard(text string) (string, []chat_app_dto.CardWithID, bool) {
	if !helpHintPattern.MatchString(text) {
		return text, nil, false
	}
	// The first line goes as message text (the Home / notification
	// preview); the card holds the rest and the menu button.
	short, rest, _ := strings.Cut(WithoutHelpHint(text), "\n")
	var widgets []chat_app_dto.Widget
	if rest = cardBody(rest); rest != "" {
		widgets = append(widgets, chat_app_dto.Widget{TextParagraph: &chat_app_dto.TextParagraph{Text: ToCardHTML(rest)}})
	}
	widgets = append(widgets, chat_app_dto.Widget{ButtonList: &chat_app_dto.ButtonList{Buttons: []chat_app_dto.Button{MenuButton()}}})
	card := chat_app_dto.Card{Sections: []chat_app_dto.Section{{Widgets: widgets}}}
	return short, []chat_app_dto.CardWithID{{CardID: "nudge-notice", Card: card}}, true
}

// NotifChoiceFunction is the welcome card's notification buttons ("on" =
// "1" turns the master switch on, "0" = later). Answered by chat_app_service
// itself, not chat_interactive.
const NotifChoiceFunction = "notif_choice"

// WelcomeCard is the install greeting: the first line as message text, the
// rest in a card with — while notifications are still off — "Nyalakan
// notifikasi" / "Nanti saja", then the menu button.
func WelcomeCard(text string, askNotif bool) (string, []chat_app_dto.CardWithID) {
	short, rest, _ := strings.Cut(WithoutHelpHint(text), "\n")
	var widgets []chat_app_dto.Widget
	if rest = cardBody(rest); rest != "" {
		widgets = append(widgets, chat_app_dto.Widget{TextParagraph: &chat_app_dto.TextParagraph{Text: ToCardHTML(rest)}})
	}
	if askNotif {
		widgets = append(widgets, chat_app_dto.Widget{ButtonList: &chat_app_dto.ButtonList{Buttons: []chat_app_dto.Button{
			actionButton("🔔 Nyalakan notifikasi", NotifChoiceFunction, "on", "1"),
			actionButton("Nanti saja", NotifChoiceFunction, "on", "0"),
		}}})
	}
	widgets = append(widgets, chat_app_dto.Widget{ButtonList: &chat_app_dto.ButtonList{Buttons: []chat_app_dto.Button{MenuButton()}}})
	card := chat_app_dto.Card{Sections: []chat_app_dto.Section{{Widgets: widgets}}}
	return short, []chat_app_dto.CardWithID{{CardID: "nudge-welcome", Card: card}}
}

var (
	boldPattern   = regexp.MustCompile(`\*([^*\n]+)\*`)
	italicPattern = regexp.MustCompile(`(^|[\s(])_([^_\n]+)_`)
	strikePattern = regexp.MustCompile(`(^|[\s(])~([^~\n]+)~`)
	urlPattern    = regexp.MustCompile(`https?://[^\s<]+[^\s<.,!?)]`)
	blankLines    = regexp.MustCompile(`\n{3,}`)
)

// ToCardHTML turns Chat markup (*bold*, _italic_, ~strike~, links, \n)
// into the HTML subset card text renders, escaping everything else.
func ToCardHTML(chatText string) string {
	s := html.EscapeString(chatText)
	s = boldPattern.ReplaceAllString(s, "<b>$1</b>")
	s = italicPattern.ReplaceAllString(s, "$1<i>$2</i>")
	s = strikePattern.ReplaceAllString(s, "$1<s>$2</s>")
	s = urlPattern.ReplaceAllStringFunc(s, func(u string) string {
		return `<a href="` + u + `">` + u + `</a>`
	})
	return strings.ReplaceAll(s, "\n", "<br>")
}

// Build is a chat_delivery.CardBuilder: the title line sent as message text
// (what Home and notifications show, e.g. "⏰ *CORE-1* — Reminder T-1 jam")
// plus one card headed by the ticket summary. Every card about a ticket
// (reminder, overage, status change) gets "Geser status" — it opens the
// ticket with a button per status its Jira workflow allows — plus Jira and
// the menu.
func Build(msg google_chat_notifier_dto.ReminderMessage) (string, []chat_app_dto.CardWithID) {
	var short, body string
	title := msg.IssueSummary
	if title == "" {
		title = msg.IssueKey
	}
	header := &chat_app_dto.CardHeader{Title: title}
	if msg.Text != "" {
		// Status change notice (AC6.2).
		short, body, _ = strings.Cut(msg.Text, "\n")
		if body == "" {
			body = short
		}
		if strings.HasPrefix(short, "🔀") && msg.IssueKey != "" {
			short = "🔀 *" + msg.IssueKey + "* — Status berubah"
		}
	} else {
		short = google_chat_notifier_service.MessageHeader(msg)
		body = google_chat_notifier_service.BuildMessageText(msg)
	}
	// The header already names the ticket and "Buka di Jira" links it: the
	// card body keeps only the details.
	body = cardBody(WithoutHelpHint(body), short, msg.IssueSummary, msg.IssueURL)

	var bs []chat_app_dto.Button
	if msg.IssueKey != "" {
		bs = append(bs, geserButton(msg.IssueKey))
	}
	if msg.IssueURL != "" {
		bs = append(bs, chat_app_dto.Button{Text: "Buka di Jira", OnClick: chat_app_dto.OnClick{OpenLink: &chat_app_dto.OpenLink{URL: msg.IssueURL}}})
	}
	bs = append(bs, MenuButton())

	widgets := []chat_app_dto.Widget{{TextParagraph: &chat_app_dto.TextParagraph{Text: ToCardHTML(body)}}}
	if len(bs) > 0 {
		widgets = append(widgets, chat_app_dto.Widget{ButtonList: &chat_app_dto.ButtonList{Buttons: bs}})
	}
	card := chat_app_dto.Card{
		Header:   header,
		Sections: []chat_app_dto.Section{{Widgets: widgets}},
	}
	return short, []chat_app_dto.CardWithID{{CardID: "nudge-" + msg.IssueKey, Card: card}}
}

// cardBody drops the lines a ticket card shows elsewhere (title line,
// summary, Jira URL) and the blank lines they leave at the top.
func cardBody(body string, drop ...string) string {
	skip := map[string]bool{}
	for _, d := range drop {
		if d = strings.TrimSpace(d); d != "" {
			skip[d] = true
		}
	}
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		if skip[strings.TrimSpace(line)] {
			continue
		}
		if len(kept) == 0 && strings.TrimSpace(line) == "" {
			continue
		}
		kept = append(kept, line)
	}
	out := strings.Join(kept, "\n")
	return strings.TrimRight(blankLines.ReplaceAllString(out, "\n\n"), "\n ")
}

// StripActions returns a copy of card without action buttons (links stay)
// and with status appended as its last section (AC6.3, AC6.4).
func StripActions(card chat_app_dto.Card, status string) chat_app_dto.Card {
	return stripButtons(card, status, func(b chat_app_dto.Button) bool { return b.OnClick.Action == nil })
}

// AnswerNotifChoice drops the welcome card's notification buttons (the menu
// button stays) and appends status as its last section.
func AnswerNotifChoice(card chat_app_dto.Card, status string) chat_app_dto.Card {
	return stripButtons(card, status, func(b chat_app_dto.Button) bool {
		return b.OnClick.Action == nil || b.OnClick.Action.Function != NotifChoiceFunction
	})
}

func stripButtons(card chat_app_dto.Card, status string, keepButton func(chat_app_dto.Button) bool) chat_app_dto.Card {
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
				if keepButton(b) {
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
