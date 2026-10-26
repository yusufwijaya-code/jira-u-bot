package google_chat_notifier_service

import (
	"context"
	"fmt"
	"net/http"

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

	payload := chatPayload{Text: buildMessageText(msg)}

	_, err = curl_helper.CurlRequest[struct{}](
		ctx, s.cfg.GoogleChatWebhookURL, nil, payload, http.MethodPost, 10000, false,
	)
	if err != nil {
		return fmt.Errorf("google_chat_notifier: SendReminder(%s) failed: %w", msg.IssueKey, err)
	}
	return nil
}

func buildMessageText(msg google_chat_notifier_dto.ReminderMessage) string {
	budgetHours := float64(msg.BudgetSeconds) / 3600
	elapsedHours := float64(msg.ElapsedSeconds) / 3600
	remaining := budgetHours - elapsedHours

	header := fmt.Sprintf("⏰ *%s* — reminder %s", msg.IssueKey, tierLabel(msg.Tier))
	remainingLine := fmt.Sprintf("Sisa: %.1f jam", remaining)
	if msg.IsOverage {
		header = fmt.Sprintf("🔴 *%s* — SUDAH LEWAT BUDGET", msg.IssueKey)
		remainingLine = fmt.Sprintf("Overage: %.1f jam", -remaining)
	}

	return fmt.Sprintf(
		"%s\n<%s|%s> — %s\nAssignee: %s\nSP: %.1f (budget %.1f jam) | Elapsed: %.1f jam | %s",
		header, msg.IssueURL, msg.IssueKey, msg.IssueSummary, msg.AssigneeName,
		msg.StoryPoints, budgetHours, elapsedHours, remainingLine,
	)
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
