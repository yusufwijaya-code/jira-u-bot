package jira_credential_dto

import "time"

// Record is one tr_usercredential row — the encrypted token is kept as raw
// bytes here too (decryption happens in jira_credential_service, never in
// the repository layer).
type Record struct {
	UserCredentialID      int64
	UserID                int64
	JiraEmail             string
	JiraAPITokenEncrypted []byte
	FlagActive            bool
	CreatedDate           time.Time
}
