package chat_app_card

import (
	"strings"
	"testing"

	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/google_chat_notifier/google_chat_notifier_dto"
)

func buttons(card chat_app_dto.Card) []chat_app_dto.Button {
	var out []chat_app_dto.Button
	for _, s := range card.Sections {
		for _, w := range s.Widgets {
			if w.ButtonList != nil {
				out = append(out, w.ButtonList.Buttons...)
			}
		}
	}
	return out
}

func TestToCardHTML(t *testing.T) {
	got := ToCardHTML("*Assignee:* A <b>\nline2")
	if got != "<b>Assignee:</b> A &lt;b&gt;<br>line2" {
		t.Errorf("ToCardHTML = %q", got)
	}
}

func TestBuild_CodeProcessReminderHasBothMovesAndJira(t *testing.T) {
	msg := google_chat_notifier_dto.ReminderMessage{IssueKey: "CORE-1", IssueURL: "https://jira/browse/CORE-1", IssueSummary: "S", Tier: "5m"}
	text, cards := Build(msg)
	if !strings.Contains(text, "CORE-1") || len(cards) != 1 {
		t.Fatalf("text=%q cards=%d", text, len(cards))
	}
	bs := buttons(cards[0].Card)
	if len(bs) != 3 || bs[0].OnClick.Action.Function != GeserFunction || bs[2].OnClick.OpenLink.URL != "https://jira/browse/CORE-1" {
		t.Errorf("buttons = %+v", bs)
	}
}

func TestBuild_ReviewReminderOnlyMovesToTodo(t *testing.T) {
	_, cards := Build(google_chat_notifier_dto.ReminderMessage{IssueKey: "CORE-1", IssueURL: "u", InReview: true})
	bs := buttons(cards[0].Card)
	if len(bs) != 2 || bs[0].OnClick.Action.Parameters[1].Value != "todo" {
		t.Errorf("buttons = %+v", bs)
	}
}

func TestBuild_StatusNoticeOnlyOpensJira(t *testing.T) {
	text, cards := Build(google_chat_notifier_dto.ReminderMessage{IssueKey: "CORE-1", IssueURL: "u", Text: "🔀 *CORE-1* — X\n*To Do* → *Code Process*"})
	bs := buttons(cards[0].Card)
	if text != "🔀 *CORE-1* — X" || len(bs) != 1 || bs[0].OnClick.OpenLink == nil {
		t.Errorf("text=%q buttons=%+v", text, bs)
	}
}

func TestStripActions_KeepsLinksAndAddsStatus(t *testing.T) {
	_, cards := Build(google_chat_notifier_dto.ReminderMessage{IssueKey: "CORE-1", IssueURL: "u"})
	out := StripActions(cards[0].Card, "✅ Digeser")
	bs := buttons(out)
	if len(bs) != 1 || bs[0].OnClick.OpenLink == nil {
		t.Errorf("buttons after strip = %+v", bs)
	}
	last := out.Sections[len(out.Sections)-1].Widgets[0].TextParagraph
	if last == nil || last.Text != "✅ Digeser" {
		t.Errorf("status widget = %+v", last)
	}
	if len(buttons(cards[0].Card)) != 3 {
		t.Error("StripActions must not mutate its input")
	}
}
