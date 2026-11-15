package admin_dashboard_dto

import "time"

// UserRow is one row in the Admin Dashboard user list (Tech Spec
// "Dashboard Jira Manage", AC6.1) — CredentialMasked is only meaningful
// when HasCredential is true, and is never the full token (AC6.4).
type UserRow struct {
	UserID           int64     `json:"userId"`
	Email            string    `json:"email"`
	UserName         string    `json:"userName"`
	FlagActive       bool      `json:"flagActive"`
	CreatedDate      time.Time `json:"createdDate"`
	JiraAccountID    string    `json:"jiraAccountId"`
	HasCredential    bool      `json:"hasCredential"`
	CredentialMasked string    `json:"credentialMasked"`
	// Tech Spec "Nudge sebagai Google Chat App" (AC4.1, AC4.4).
	ChatAppActivated     bool       `json:"chatAppActivated"`
	ChatAppActivatedDate *time.Time `json:"chatAppActivatedDate"`
	ChatSender           *string    `json:"chatSender"`
	EffectiveChatSender  string     `json:"effectiveChatSender"`
}

// BroadcastResult reports the outcome of BroadcastMessage — FailedEmails is
// nil (not just empty) when every send succeeded, so callers/templates can
// tell "nothing failed" apart from "list not populated" at a glance.
type BroadcastResult struct {
	SentCount    int
	FailedEmails []string
}

// UserChange is a write an admin made to another user's settings (Tech
// Spec "Admin Panel Revamp", AC4.1). Kind is one of "notification",
// "reset", "mute", "unmute", "credential_updated", "credential_deleted";
// BoardID 0 = the default scope / global mute; Until is set for "mute".
type UserChange struct {
	Kind    string
	BoardID int64
	Until   *time.Time
}
