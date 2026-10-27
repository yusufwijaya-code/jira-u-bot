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
