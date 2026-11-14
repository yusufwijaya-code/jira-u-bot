package reminder_log_dto

import "time"

type ReminderLog struct {
	ReminderLogID int64
	IssueKey      string
	Tier          string
	SentDate      time.Time
}

const (
	Tier2h      = "2h"
	Tier1h      = "1h"
	Tier30m     = "30m"
	Tier15m     = "15m"
	Tier5m      = "5m"
	Tier1m      = "1m"
	TierOverage = "overage"
)

// OverageTierPrefix marks a post-budget tier in tr_reminderlog.tier ("+15m")
// so it never collides with the pre-budget tier of the same length ("15m").
const OverageTierPrefix = "+"

// DeliveryStatus values for tr_reminderlog.deliveryStatus. Both count as
// "handled" for dedup — a skipped tier (muted / quiet hours / superseded by
// a more relevant tier) is never sent later.
const (
	DeliverySent    = "sent"
	DeliverySkipped = "skipped"
)
