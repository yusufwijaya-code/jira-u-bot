package user_dashboard_handler

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/user_setting/user_setting_dto"
)

// selfNoticeTimeout bounds one confirmation DM, which runs detached from
// the user's request.
const selfNoticeTimeout = 10 * time.Second

// PlainTextDM sends a plain DM through the normal delivery path —
// chat_delivery.Router (Nudge App when activated, else the operator).
type PlainTextDM interface {
	SendPlainTextDM(ctx context.Context, email, text string) error
}

// BoardDescriber names a board — user_dashboard_service.UserDashboardService.
type BoardDescriber interface {
	DescribeBoard(ctx context.Context, boardID int64) (name, projectKey string)
}

// SelfChangeListener confirms a user's own settings change on the web with
// a short DM (the counterpart of the admin change notice). Best-effort:
// one DM per save, nothing for a no-op, sent in the background and only
// logged on failure. Chat commands don't come through here (chat replies
// itself); the sender switch has its own confirmation DM.
func SelfChangeListener(dm PlainTextDM, boards BoardDescriber) ChangeListener {
	return (&selfNotifier{dm: dm, boards: boards}).listen
}

type selfNotifier struct {
	dm     PlainTextDM
	boards BoardDescriber
	// sendSync runs the DM inline (tests).
	sendSync bool
}

func (n *selfNotifier) listen(_ *gin.Context, target SettingsTarget, ch SettingsChange) {
	if n.dm == nil || ch.NoOp || ch.Kind == ChangeChatSender || target.Email == "" {
		return
	}
	send := func() {
		ctx, cancel := context.WithTimeout(context.Background(), selfNoticeTimeout)
		defer cancel()
		boardName := ""
		if ch.BoardID != user_setting_dto.DefaultBoardID {
			boardName = fmt.Sprintf("Board #%d", ch.BoardID)
			if n.boards != nil {
				if name, _ := n.boards.DescribeBoard(ctx, ch.BoardID); name != "" {
					boardName = name
				}
			}
		}
		text := SelfChangeNoticeText(boardName, ch)
		if text == "" {
			return
		}
		if err := n.dm.SendPlainTextDM(ctx, target.Email, text); err != nil {
			log.Printf("user_dashboard: self change DM to %s failed: %v", target.Email, err)
		}
	}
	if n.sendSync {
		send()
		return
	}
	go send()
}

const selfNoticePrefix = "✅ Pengaturan kamu diperbarui"

var (
	shortDay   = [...]string{"Min", "Sen", "Sel", "Rab", "Kam", "Jum", "Sab"}
	shortMonth = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
)

// noticeItem is one line of a confirmation: "label: value", or a bare
// sentence when label is empty.
type noticeItem struct{ label, value string }

// SelfChangeNoticeText is the confirmation DM for the user's own change;
// boardName is empty for the default scope / global mute. "" = nothing to
// say (the change is a no-op).
func SelfChangeNoticeText(boardName string, ch SettingsChange) string {
	scope := boardName
	if scope == "" {
		scope = "default semua board"
	}
	switch ch.Kind {
	case ChangeNotificationsSwitch:
		if ch.Enabled != nil && !*ch.Enabled {
			return selfNoticePrefix + ": Notifikasi otomatis dimatikan"
		}
		return selfNoticePrefix + ": Notifikasi otomatis dinyalakan"
	case ChangeCredentialUpdated:
		return selfNoticePrefix + ": Jira credential disimpan"
	case ChangeCredentialDeleted:
		return selfNoticePrefix + ": Jira credential dihapus"
	case ChangeReset:
		return selfNoticePrefix + ": " + scope + " kembali mengikuti pengaturan default"
	case ChangeMute:
		what := "semua notifikasi"
		if boardName != "" {
			what = boardName
		}
		if ch.Until == nil || user_setting_dto.IsForever(ch.Until) {
			return selfNoticePrefix + ": Mute " + what + " sampai kamu nyalakan lagi"
		}
		return selfNoticePrefix + ": Mute " + what + " sampai " + formatWhen(*ch.Until)
	case ChangeUnmute:
		if boardName != "" {
			return selfNoticePrefix + ": Mute " + boardName + " dicabut, notifikasi board ini aktif lagi"
		}
		return selfNoticePrefix + ": Mute semua notifikasi dicabut"
	case ChangeNotification:
		return notificationNotice(boardName, scope, ch)
	}
	return ""
}

func notificationNotice(boardName, scope string, ch SettingsChange) string {
	if ch.After == nil {
		return selfNoticePrefix + ": Pengaturan notifikasi (" + scope + ") disimpan"
	}
	var items []noticeItem
	becameCustom := boardName != "" && ch.Before != nil && ch.Before.UseDefault
	if becameCustom {
		items = append(items, noticeItem{value: boardName + " sekarang pakai pengaturan khusus"})
	}
	items = append(items, notificationDiff(ch.Before, *ch.After)...)
	switch len(items) {
	case 0:
		return ""
	case 1:
		it := items[0]
		if it.label == "" {
			return selfNoticePrefix + ": " + it.value
		}
		return fmt.Sprintf("%s: %s (%s): %s", selfNoticePrefix, it.label, scope, it.value)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s):", selfNoticePrefix, scope)
	for _, it := range items {
		b.WriteString("\n• ")
		if it.label != "" {
			b.WriteString(it.label + ": ")
		}
		b.WriteString(it.value)
	}
	return b.String()
}

// notificationDiff lists the fields that differ (every field when before is
// unknown).
func notificationDiff(before *user_setting_dto.UserSetting, after user_setting_dto.UserSetting) []noticeItem {
	var items []noticeItem
	changed := func(same bool) bool { return before == nil || !same }
	if changed(before != nil && slices.Equal(before.ReminderTiers, after.ReminderTiers)) {
		items = append(items, noticeItem{"Reminder sebelum budget habis", tierList(after.ReminderTiers, "T-")})
	}
	if changed(before != nil && slices.Equal(before.OverageTiers, after.OverageTiers)) {
		items = append(items, noticeItem{"Reminder setelah budget habis", tierList(after.OverageTiers, "T+")})
	}
	if changed(before != nil && sameIntPtr(before.OverageRepeatMinutes, after.OverageRepeatMinutes)) {
		v := "berhenti"
		if after.OverageRepeatMinutes != nil {
			v = fmt.Sprintf("tiap %d menit", *after.OverageRepeatMinutes)
		}
		items = append(items, noticeItem{"Ulang setelah tier terakhir", v})
	}
	if changed(before != nil && before.NotifyStatusChange == after.NotifyStatusChange &&
		slices.Equal(before.StatusChangeStatuses, after.StatusChangeStatuses)) {
		v := "mati"
		if after.NotifyStatusChange {
			v = "nyala (semua status)"
			if len(after.StatusChangeStatuses) > 0 {
				v = "nyala (" + strings.Join(after.StatusChangeStatuses, ", ") + ")"
			}
		}
		items = append(items, noticeItem{"Notifikasi perubahan status", v})
	}
	if changed(before != nil && sameQuiet(before.QuietHours, after.QuietHours)) {
		v := "mati"
		if q := after.QuietHours; q != nil {
			v = strings.ReplaceAll(q.Start, ":", ".") + "–" + strings.ReplaceAll(q.End, ":", ".") + " WIB"
		}
		items = append(items, noticeItem{"Jam tenang harian", v})
	}
	return items
}

// SameNotification reports whether two settings carry the same
// notification fields (mute and scope aside).
func SameNotification(a, b user_setting_dto.UserSetting) bool {
	return slices.Equal(a.ReminderTiers, b.ReminderTiers) &&
		slices.Equal(a.OverageTiers, b.OverageTiers) &&
		sameIntPtr(a.OverageRepeatMinutes, b.OverageRepeatMinutes) &&
		a.NotifyStatusChange == b.NotifyStatusChange &&
		slices.Equal(a.StatusChangeStatuses, b.StatusChangeStatuses) &&
		sameQuiet(a.QuietHours, b.QuietHours)
}

func sameIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameQuiet(a, b *user_setting_dto.QuietHours) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// tierList renders tier codes as "T-2 jam, T-1 jam, T-30 menit"; "mati"
// when empty.
func tierList(codes []string, prefix string) string {
	if len(codes) == 0 {
		return "mati"
	}
	out := make([]string, len(codes))
	for i, code := range codes {
		out[i] = prefix + tierLabel(code)
	}
	return strings.Join(out, ", ")
}

// tierLabel mirrors the web's tierDuration: "45m" → "45 menit", "2h" →
// "2 jam", "90m" → "1,5 jam".
func tierLabel(code string) string {
	minutes := int(user_setting_dto.TierDuration(code) / time.Minute)
	if minutes < 60 || minutes%30 != 0 {
		return fmt.Sprintf("%d menit", minutes)
	}
	if minutes%60 == 0 {
		return fmt.Sprintf("%d jam", minutes/60)
	}
	return fmt.Sprintf("%d,5 jam", minutes/60)
}

// formatWhen renders "Sel, 17 Nov 23.58 WIB".
func formatWhen(t time.Time) string {
	w := t.In(user_setting_dto.Jakarta)
	return fmt.Sprintf("%s, %d %s %02d.%02d WIB", shortDay[w.Weekday()], w.Day(), shortMonth[w.Month()-1], w.Hour(), w.Minute())
}
