package status_watch_service

import (
	"fmt"
	"strings"
	"time"

	"nudge/modules/google_chat_notifier/google_chat_notifier_service"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/user_setting/user_setting_dto"
)

// statusChangeText renders the DM (Tech Spec AC1.3, AC2.2):
//
//	🔀 *OMS-1* — Summary
//	*To Do* → *Code Process* · oleh Anwar · 10:15 WIB
//	⏱ ... (only when leaving a tracked status)
//	https://.../browse/OMS-1
func statusChangeText(key, summary, url, from, to, actor string, at time.Time, timing string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔀 *%s* — %s\n*%s* → *%s*", key, summary, from, to)
	if actor != "" {
		fmt.Fprintf(&b, " · oleh %s", actor)
	}
	fmt.Fprintf(&b, " · %s WIB", at.In(user_setting_dto.Jakarta).Format("15:04"))
	if timing != "" {
		b.WriteString("\n" + timing)
	}
	if url != "" {
		b.WriteString("\n" + url)
	}
	return b.String()
}

// timingLine is the budget line for a ticket that just left its tracked
// statuses (replaces the old "finished early" / "back to To Do" notices).
func timingLine(rule jira_client_dto.TrackingRule, to string, elapsed, budget int64) string {
	if budget <= 0 {
		return ""
	}
	used := google_chat_notifier_service.FormatDuration(elapsed)
	total := google_chat_notifier_service.FormatDuration(budget)
	switch {
	case elapsed > budget:
		return fmt.Sprintf("⏱ Waktu terpakai %s dari budget %s (lewat %s)", used, total, google_chat_notifier_service.FormatDuration(elapsed-budget))
	case rule.IsDone(to):
		return fmt.Sprintf("⏱ Selesai %s lebih awal dari budget %s", google_chat_notifier_service.FormatDuration(budget-elapsed), total)
	default:
		return fmt.Sprintf("⏱ Waktu terpakai %s dari budget %s", used, total)
	}
}

// lastStatusAuthor returns who made the latest status change and when.
func lastStatusAuthor(histories []jira_client_dto.ChangelogEntry) (actor string, at time.Time) {
	for _, h := range histories {
		for _, item := range h.Items {
			if item.Field != "status" {
				continue
			}
			t, err := time.Parse("2006-01-02T15:04:05.000-0700", h.Created)
			if err != nil || !t.After(at) {
				continue
			}
			at = t
			actor = ""
			if h.Author != nil {
				actor = h.Author.DisplayName
			}
		}
	}
	return actor, at
}
