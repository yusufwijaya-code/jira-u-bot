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

func param(b chat_app_dto.Button, key string) string {
	for _, p := range b.OnClick.Action.Parameters {
		if p.Key == key {
			return p.Value
		}
	}
	return ""
}

// assertTicketButtons: Geser status (workflow choices), Jira, menu.
func assertTicketButtons(t *testing.T, card chat_app_dto.Card, key, url string) {
	t.Helper()
	bs := buttons(card)
	if len(bs) != 3 {
		t.Fatalf("buttons = %+v", bs)
	}
	if bs[0].OnClick.Action.Function != "nudge.ticket" || param(bs[0], "key") != key {
		t.Errorf("geser button = %+v", bs[0])
	}
	if bs[1].OnClick.OpenLink == nil || bs[1].OnClick.OpenLink.URL != url {
		t.Errorf("jira button = %+v", bs[1])
	}
	if bs[2].OnClick.Action.Function != "nudge.cmd" || param(bs[2], "cmd") != "/n-help" {
		t.Errorf("menu button = %+v", bs[2])
	}
}

func cardText(card chat_app_dto.Card) string {
	var out []string
	for _, s := range card.Sections {
		for _, w := range s.Widgets {
			if w.TextParagraph != nil {
				out = append(out, w.TextParagraph.Text)
			}
		}
	}
	return strings.Join(out, "\n")
}

func TestToCardHTML(t *testing.T) {
	got := ToCardHTML("*Assignee:* A <b>\nline2")
	if got != "<b>Assignee:</b> A &lt;b&gt;<br>line2" {
		t.Errorf("ToCardHTML = %q", got)
	}
}

func TestBuild_ReminderHasGeserJiraAndMenuButHintIsGone(t *testing.T) {
	msg := google_chat_notifier_dto.ReminderMessage{IssueKey: "CORE-1", IssueURL: "https://jira/browse/CORE-1", IssueSummary: "S", Tier: "5m"}
	text, cards := Build(msg)
	if !strings.Contains(text, "CORE-1") || len(cards) != 1 {
		t.Fatalf("text=%q cards=%d", text, len(cards))
	}
	assertTicketButtons(t, cards[0].Card, "CORE-1", "https://jira/browse/CORE-1")
	if strings.Contains(cardText(cards[0].Card), "n-help") {
		t.Errorf("the /n-help hint must become the menu button:\n%s", cardText(cards[0].Card))
	}
}

func TestBuild_OverageReminderAlsoGetsTheButtons(t *testing.T) {
	_, cards := Build(google_chat_notifier_dto.ReminderMessage{IssueKey: "OMS-7", IssueURL: "u", IssueSummary: "S", IsOverage: true})
	assertTicketButtons(t, cards[0].Card, "OMS-7", "u")
	if strings.Contains(cardText(cards[0].Card), "n-help") {
		t.Error("overage card must not show the /n-help hint")
	}
}

// The title line goes as message text (Home / notification preview); the
// card header is the ticket summary, so nothing is said twice.
func TestBuild_TitleLineIsTheTextAndTheHeaderIsTheSummary(t *testing.T) {
	text, cards := Build(google_chat_notifier_dto.ReminderMessage{IssueKey: "CORE-1", IssueSummary: "S", IsOverage: true})
	if text != "🔴 *CORE-1* — SUDAH LEWAT BUDGET" {
		t.Errorf("overage text = %q", text)
	}
	if h := cards[0].Card.Header; h.Title != "S" || h.Subtitle != "" {
		t.Errorf("overage header = %+v", h)
	}
	text, cards = Build(google_chat_notifier_dto.ReminderMessage{IssueKey: "CORE-1", Tier: "5m"})
	if !strings.HasPrefix(text, "⏰ *CORE-1* — Reminder") || cards[0].Card.Header.Title != "CORE-1" {
		t.Errorf("tier text = %q header = %q (no summary → key)", text, cards[0].Card.Header.Title)
	}
}

func TestBuild_StatusNoticeAlsoGetsTheButtons(t *testing.T) {
	text, cards := Build(google_chat_notifier_dto.ReminderMessage{IssueKey: "CORE-1", IssueURL: "u", Text: "🔀 *CORE-1* — X\n*To Do* → *Code Process*"})
	if text != "🔀 *CORE-1* — Status berubah" {
		t.Errorf("text=%q", text)
	}
	assertTicketButtons(t, cards[0].Card, "CORE-1", "u")
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

func TestTextCard_HelpHintBecomesAMenuButton(t *testing.T) {
	short, cards, ok := TextCard("🎉 Halo! Registrasi berhasil.\n\nisi\n\n💡 Ketik */n-help* di sini buat lihat command lain")
	if !ok || short != "🎉 Halo! Registrasi berhasil." || len(cards) != 1 {
		t.Fatalf("ok=%v short=%q cards=%d", ok, short, len(cards))
	}
	// The first line is the message text; the card holds the rest.
	if txt := cardText(cards[0].Card); txt != "isi" {
		t.Errorf("card text = %q", txt)
	}
	if bs := buttons(cards[0].Card); len(bs) != 1 || param(bs[0], "cmd") != "/n-help" {
		t.Errorf("buttons = %+v", bs)
	}
	if _, _, ok := TextCard("pesan biasa"); ok {
		t.Error("text without the hint must stay plain")
	}
}

func TestToCardHTML_ItalicLinksAndStrike(t *testing.T) {
	got := ToCardHTML("Atur di *Pengaturan*: https://nudge.labcup.tech/dashboard/settings.\n_Pesan dicatat._ ~lama~ file_name_x")
	want := `Atur di <b>Pengaturan</b>: <a href="https://nudge.labcup.tech/dashboard/settings">https://nudge.labcup.tech/dashboard/settings</a>.<br><i>Pesan dicatat.</i> <s>lama</s> file_name_x`
	if got != want {
		t.Errorf("ToCardHTML =\n%q\nwant\n%q", got, want)
	}
}

func TestWithoutHelpHint_LeavesNoDoubleGap(t *testing.T) {
	got := WithoutHelpHint("A\n\n💡 Ketik */n-help* buat lihat command\n\n_B_")
	if got != "A\n\n_B_" {
		t.Errorf("got %q", got)
	}
}

func TestBuild_CardBodyHasOnlyTheDetails(t *testing.T) {
	_, cards := Build(google_chat_notifier_dto.ReminderMessage{IssueKey: "CORE-1", IssueURL: "https://jira/browse/CORE-1", IssueSummary: "Fix login", AssigneeName: "Yusuf", Tier: "1h"})
	body := cardText(cards[0].Card)
	if strings.Contains(body, "Reminder") || strings.Contains(body, "Fix login") || strings.Contains(body, "https://jira") || !strings.HasPrefix(body, "<b>Assignee:</b>") {
		t.Errorf("reminder body repeats the header/link:\n%s", body)
	}
	_, cards = Build(google_chat_notifier_dto.ReminderMessage{IssueKey: "OMS-7", IssueURL: "https://jira/browse/OMS-7", IssueSummary: "S", Text: "🔀 *OMS-7* — S\n*To Do* → *Code Process* · 10:15 WIB\nhttps://jira/browse/OMS-7"})
	if h := cards[0].Card.Header.Title; h != "S" {
		t.Errorf("status title = %q", h)
	}
	if body := cardText(cards[0].Card); body != "<b>To Do</b> → <b>Code Process</b> · 10:15 WIB" {
		t.Errorf("status body = %q", body)
	}
}
