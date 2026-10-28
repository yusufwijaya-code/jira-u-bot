package google_chat_notifier_service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"jira-u-bot/base/helpers/curl_helper"
	"jira-u-bot/config"
	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_dto"
)

type googleChatNotifierServiceImpl struct {
	cfg *config.AppConfig
}

func NewGoogleChatNotifierService(cfg *config.AppConfig) GoogleChatNotifierService {
	return &googleChatNotifierServiceImpl{cfg: cfg}
}

type chatPayload struct {
	Text string `json:"text"`
}

func (s *googleChatNotifierServiceImpl) SendReminder(ctx context.Context, msg google_chat_notifier_dto.ReminderMessage) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("google_chat_notifier: recovered from panic: %v", r)
		}
	}()

	if s.cfg.GoogleChatWebhookURL == "" {
		return nil // no webhook configured — skip, do not fail the poll cycle
	}

	payload := chatPayload{Text: BuildMessageText(msg)}

	_, err = curl_helper.CurlRequest[struct{}](
		ctx, s.cfg.GoogleChatWebhookURL, nil, payload, http.MethodPost, 10000, false,
	)
	if err != nil {
		return fmt.Errorf("google_chat_notifier: SendReminder(%s) failed: %w", msg.IssueKey, err)
	}
	return nil
}

// messageFooter is appended to every reminder/overage/early-completion
// message — plain URLs (no <url|label> masking) and this footer are the
// only requirements shared across all three shapes.
const messageFooter = "#BersamaSayaMenempuhOKRyangPassss"

// helpHint advertises the interactive slash commands on every outbound
// message — this function is shared by both the webhook/space path and the
// DM path, so adding it here covers both without touching either notifier
// separately. Commands are typed directly into the DM conversation (no
// separate bot to address — see chat_command_poller), hence "di sini" and
// not "ke bot ini".
const helpHint = "💡 Ketik */u-help* di sini buat lihat command lain"

func messageHeader(msg google_chat_notifier_dto.ReminderMessage) string {
	switch {
	case msg.IsEarlyCompletion:
		return fmt.Sprintf("✅ *%s* — Selesai Lebih Awal", msg.IssueKey)
	case msg.IsOverage:
		return fmt.Sprintf("🔴 *%s* — SUDAH LEWAT BUDGET", msg.IssueKey)
	default:
		return fmt.Sprintf("⏰ *%s* — Reminder %s", msg.IssueKey, tierLabel(msg.Tier))
	}
}

// BuildMessageText renders a plain, non-masked URL on its own line instead
// of the Hangouts-style "<url|label>" hyperlink syntax: that syntax is only
// parsed when a Chat app posts a message (the webhook/space path), not when
// a message is sent via the Chat API under a user's own OAuth token (the DM
// path) — using it produced one rendering in the shared space and a
// mangled, line-break-less wall of text in DMs. A bare URL auto-linkifies
// identically in both paths.
func BuildMessageText(msg google_chat_notifier_dto.ReminderMessage) string {
	budgetHours := float64(msg.BudgetSeconds) / 3600
	elapsedHours := float64(msg.ElapsedSeconds) / 3600
	remainingSeconds := msg.BudgetSeconds - msg.ElapsedSeconds

	lines := []string{
		messageHeader(msg),
		msg.IssueURL,
		msg.IssueSummary,
		"",
		fmt.Sprintf("*Assignee:* %s", msg.AssigneeName),
		fmt.Sprintf("*Masuk Code Process:* %s", FormatDeadline(msg.CodeProcessStartedAt)),
	}
	if msg.IsEarlyCompletion {
		lines = append(lines, fmt.Sprintf("*Pindah ke Code Review:* %s", FormatDeadline(msg.CodeReviewEnteredAt)))
	}
	lines = append(lines,
		fmt.Sprintf("*Story Point:* %.1f SP (budget %.1f jam, dihitung dari %.1f SP × 1 jam/SP)", msg.StoryPoints, budgetHours, msg.StoryPoints),
		fmt.Sprintf("*Waktu berjalan:* %.1f jam", elapsedHours),
	)

	switch {
	case msg.IsEarlyCompletion:
		lines = append(lines,
			fmt.Sprintf("*Sisa budget tidak terpakai:* %s", FormatDuration(remainingSeconds)),
			fmt.Sprintf("*Limit seharusnya selesai:* %s", FormatDeadline(msg.Deadline)),
			"",
			"Mantap, kerja bagus! 👏",
		)
	case msg.IsOverage:
		lines = append(lines,
			fmt.Sprintf("*Lewat budget selama:* %s", FormatDuration(remainingSeconds)),
			"",
			"Segera selesaikan atau koordinasikan ke lead ya 🙏",
		)
	default:
		lines = append(lines,
			fmt.Sprintf("*Sisa waktu:* %s", FormatDuration(remainingSeconds)),
			fmt.Sprintf("*Harus Selesai Pass:* %s", FormatDeadline(msg.Deadline)),
		)
	}

	lines = append(lines, "", helpHint, "", messageFooter)
	return strings.Join(lines, "\n")
}

// FormatDuration renders a second count as "D hari H jam M menit S detik" —
// used for both the pre-deadline countdown and the overage amount, so both
// read in the same unit instead of a vague decimal-hours figure.
func FormatDuration(totalSeconds int64) string {
	if totalSeconds < 0 {
		totalSeconds = -totalSeconds
	}
	days := totalSeconds / 86400
	hours := (totalSeconds % 86400) / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%d hari %d jam %d menit %d detik", days, hours, minutes, seconds)
}

var indonesianWeekdays = map[time.Weekday]string{
	time.Sunday: "Minggu", time.Monday: "Senin", time.Tuesday: "Selasa",
	time.Wednesday: "Rabu", time.Thursday: "Kamis", time.Friday: "Jumat", time.Saturday: "Sabtu",
}

var indonesianMonths = map[time.Month]string{
	time.January: "Jan", time.February: "Feb", time.March: "Mar", time.April: "Apr",
	time.May: "Mei", time.June: "Jun", time.July: "Jul", time.August: "Agu",
	time.September: "Sep", time.October: "Okt", time.November: "Nov", time.December: "Des",
}

// FormatDeadline renders an absolute instant as "Senin, 26 Okt 2026
// 15:12:00 WIB" — Go's time package has no Indonesian locale, so day/month
// names are looked up from a small fixed table instead of pulling in an
// i18n dependency for this one line.
func FormatDeadline(t time.Time) string {
	return fmt.Sprintf("%s, %02d %s %d %02d:%02d:%02d WIB",
		indonesianWeekdays[t.Weekday()], t.Day(), indonesianMonths[t.Month()], t.Year(),
		t.Hour(), t.Minute(), t.Second())
}

func tierLabel(tier string) string {
	switch tier {
	case "2h":
		return "T-2 jam"
	case "1h":
		return "T-1 jam"
	case "30m":
		return "T-30 menit"
	case "15m":
		return "T-15 menit"
	case "5m":
		return "T-5 menit"
	case "1m":
		return "T-1 menit"
	default:
		return tier
	}
}
