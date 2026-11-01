package admin_dashboard_dto

import "time"

// UserRow is one row in the Admin Dashboard user list (Tech Spec
// "Dashboard Jira Manage", AC6.1) — CredentialMasked is only meaningful
// when HasCredential is true, and is never the full token (AC6.4).
type UserRow struct {
	UserID           int64
	Email            string
	UserName         string
	FlagActive       bool
	CreatedDate      time.Time
	JiraAccountID    string
	HasCredential    bool
	CredentialMasked string
}
