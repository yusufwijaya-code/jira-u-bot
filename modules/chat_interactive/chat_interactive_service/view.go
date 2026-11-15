package chat_interactive_service

import (
	"strings"

	"nudge/modules/chat_app/chat_app_card"
	"nudge/modules/chat_app/chat_app_dto"
)

// view is one reply, rendered both as a Chat card and as plain text, so
// the card and its text version never drift apart.
type view struct {
	id       string // cardId
	title    string
	subtitle string
	sections []section
	// tip is shown in the text rendering only: how to do by typing what
	// the card's buttons do.
	tip string
}

type section struct {
	header string
	// collapseAfter > 0 folds the section after that many items.
	collapseAfter int
	items         []item
}

// item is one widget: a paragraph, a row or a set of buttons.
type item struct {
	text    string // Chat markup: *bold*, _italic_, \n
	row     *row
	buttons []button
	date    *dateField
	input   *inputField
}

type row struct {
	top, main, bottom string // Chat markup
	button            *button
	// depth > 0 shows the row as a branch (↳, indented) of the row above.
	depth int
}

// branchPrefix is the card HTML put before a branch row's labels.
func branchPrefix(depth int) string {
	if depth <= 0 {
		return ""
	}
	return strings.Repeat("&emsp;", depth-1) + "↳ "
}

type button struct {
	label  string
	fn     string // action function (without ActionPrefix)
	params [][2]string
	url    string
}

type dateField struct{ name, label string }
type inputField struct{ name, label, hint string }

func paragraph(text string) item { return item{text: text} }
func buttons(bs ...button) item  { return item{buttons: bs} }
func action(label, fn string, params ...string) button {
	b := button{label: label, fn: fn}
	for i := 0; i+1 < len(params); i += 2 {
		b.params = append(b.params, [2]string{params[i], params[i+1]})
	}
	return b
}
func link(label, url string) button { return button{label: label, url: url} }

// cardHTML turns Chat markup into the HTML subset card text renders — the
// same rules as the automatic notices (bold, italic, strike, links).
func cardHTML(s string) string { return chat_app_card.ToCardHTML(s) }

func (b button) widget() chat_app_dto.Button {
	if b.url != "" {
		return chat_app_dto.Button{Text: b.label, OnClick: chat_app_dto.OnClick{OpenLink: &chat_app_dto.OpenLink{URL: b.url}}}
	}
	params := []chat_app_dto.ActionParameter{{Key: "label", Value: b.label}}
	for _, p := range b.params {
		params = append(params, chat_app_dto.ActionParameter{Key: p[0], Value: p[1]})
	}
	return chat_app_dto.Button{Text: b.label, OnClick: chat_app_dto.OnClick{Action: &chat_app_dto.OnClickAction{
		Function: ActionPrefix + b.fn, Parameters: params,
	}}}
}

func (v view) card() chat_app_dto.CardWithID {
	// No card header: the message text above the card (Reply.Title) is the
	// title line, so the Home preview and notifications show it too.
	c := chat_app_dto.Card{}
	for _, s := range v.sections {
		out := chat_app_dto.Section{Header: cardHTML(s.header)}
		visible := 0 // widgets shown before "Show more"
		for i, it := range s.items {
			// A divider between two rows keeps tickets visually apart.
			if i > 0 && it.row != nil && s.items[i-1].row != nil {
				out.Widgets = append(out.Widgets, chat_app_dto.Widget{Divider: &struct{}{}})
			}
			out.Widgets = append(out.Widgets, it.widget())
			if i == s.collapseAfter-1 {
				visible = len(out.Widgets)
			}
		}
		if s.collapseAfter > 0 && len(s.items) > s.collapseAfter+1 {
			out.Collapsible, out.UncollapsibleWidgetsCount = true, visible
		}
		if len(out.Widgets) > 0 {
			c.Sections = append(c.Sections, out)
		}
	}
	return chat_app_dto.CardWithID{CardID: v.id, Card: c}
}

func (it item) widget() chat_app_dto.Widget {
	switch {
	case it.row != nil:
		pad := branchPrefix(it.row.depth)
		d := &chat_app_dto.DecoratedText{
			TopLabel: pad + cardHTML(it.row.top), Text: pad + cardHTML(it.row.main),
			BottomLabel: cardHTML(it.row.bottom), WrapText: true,
		}
		if pad != "" && d.BottomLabel != "" {
			d.BottomLabel = strings.Repeat("&emsp;", it.row.depth) + d.BottomLabel
		}
		if d.TopLabel == pad {
			d.TopLabel = ""
		}
		if it.row.button != nil {
			b := it.row.button.widget()
			d.Button = &b
		}
		return chat_app_dto.Widget{DecoratedText: d}
	case len(it.buttons) > 0:
		bs := make([]chat_app_dto.Button, 0, len(it.buttons))
		for _, b := range it.buttons {
			bs = append(bs, b.widget())
		}
		return chat_app_dto.Widget{ButtonList: &chat_app_dto.ButtonList{Buttons: bs}}
	case it.date != nil:
		return chat_app_dto.Widget{DateTimePicker: &chat_app_dto.DateTimePicker{
			Name: it.date.name, Label: it.date.label, Type: chat_app_dto.PickerDateOnly,
		}}
	case it.input != nil:
		return chat_app_dto.Widget{TextInput: &chat_app_dto.TextInput{
			Name: it.input.name, Label: it.input.label, HintText: it.input.hint,
		}}
	}
	return chat_app_dto.Widget{TextParagraph: &chat_app_dto.TextParagraph{Text: cardHTML(it.text)}}
}

// text is the plain Chat rendering (buttons and form fields are dropped;
// tip says how to type them instead).
func (v view) text() string {
	var b strings.Builder
	b.WriteString("*" + v.title + "*")
	if v.subtitle != "" {
		b.WriteString("\n" + v.subtitle)
	}
	for _, s := range v.sections {
		var lines []string
		for i, it := range s.items {
			switch {
			case it.row != nil:
				indent := strings.Repeat("      ", it.row.depth)
				bullet := "• "
				if it.row.depth > 0 {
					bullet = "↳ "
				}
				line := indent + bullet + it.row.main
				if i > 0 && s.items[i-1].row != nil {
					line = "\n" + line // blank line between tickets
				}
				if it.row.top != "" {
					line += "\n   " + indent + it.row.top
				}
				if it.row.bottom != "" {
					line += "\n   " + indent + it.row.bottom
				}
				lines = append(lines, line)
			case it.text != "":
				lines = append(lines, it.text)
			}
		}
		if len(lines) == 0 {
			continue
		}
		b.WriteString("\n\n")
		if s.header != "" {
			b.WriteString("*" + s.header + "*\n")
		}
		b.WriteString(strings.Join(lines, "\n"))
	}
	if v.tip != "" {
		b.WriteString("\n\n💡 " + v.tip)
	}
	return b.String()
}

func (v view) reply() Reply {
	return Reply{Text: v.text(), Cards: []chat_app_dto.CardWithID{v.card()}, Title: v.titleLine()}
}

// titleLine is the one line sent as message text above the card:
// "*📋 Tiket Saya* — ESB Core Board · 18 tiket".
func (v view) titleLine() string {
	line := "*" + v.title + "*"
	if v.subtitle != "" {
		line += " — " + v.subtitle
	}
	return line
}

// note is a short reply without buttons (errors, confirmations).
func note(title, body string) view {
	return view{id: "nudge-note", title: title, sections: []section{{items: []item{paragraph(body)}}}}
}
