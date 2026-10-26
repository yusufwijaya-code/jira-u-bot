package google_chat_notifier_service

import (
	"context"
	"fmt"
	"net/http"
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

func BuildMessageText(msg google_chat_notifier_dto.ReminderMessage) string {
	budgetHours := float64(msg.BudgetSeconds) / 3600
	elapsedHours := float64(msg.ElapsedSeconds) / 3600
	remainingSeconds := msg.BudgetSeconds - msg.ElapsedSeconds

	header := fmt.Sprintf("⏰ *%s* — reminder %s", msg.IssueKey, tierLabel(msg.Tier))
	timeLine := fmt.Sprintf("Sisa: %s", formatDuration(remainingSeconds))
	deadlineLine := fmt.Sprintf("Harus selesai: %s\n", formatDeadline(msg.Deadline))
	if msg.IsOverage {
		header = fmt.Sprintf("🔴 *%s* — SUDAH LEWAT BUDGET", msg.IssueKey)
		timeLine = fmt.Sprintf("Overage: %s", formatDuration(remainingSeconds))
		deadlineLine = "" // already past due — a "must finish by" instant no longer applies
	}

	return fmt.Sprintf(
		"%s\n<%s|%s> — %s\nAssignee: %s\nSP: %.1f (budget %.1f jam) | Elapsed: %.1f jam\n%s%s",
		header, msg.IssueURL, msg.IssueKey, msg.IssueSummary, msg.AssigneeName,
		msg.StoryPoints, budgetHours, elapsedHours, deadlineLine, timeLine,
	)
}

// formatDuration renders a second count as "D hari H jam M menit S detik" —
// used for both the pre-deadline countdown and the overage amount, so both
// read in the same unit instead of a vague decimal-hours figure.
func formatDuration(totalSeconds int64) string {
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

// formatDeadline renders an absolute instant as "Senin, 26 Okt 2026
// 15:12:00 WIB" — Go's time package has no Indonesian locale, so day/month
// names are looked up from a small fixed table instead of pulling in an
// i18n dependency for this one line.
func formatDeadline(t time.Time) string {
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
	case "1m":
		return "T-1 menit"
	default:
		return tier
	}
}
