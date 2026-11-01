package jira_credential_repository

import (
	"context"

	"jira-u-bot/modules/jira_credential/jira_credential_dto"
)

type JiraCredentialRepository interface {
	// Upsert creates or overwrites userID's personal credential (spec
	// AC5.5 "reset" — a second call for the same user replaces the first,
	// never adds a second row; uq_tr_usercredential_userid enforces this).
	Upsert(ctx context.Context, userID int64, jiraEmail string, encrypted []byte) error
	FindByUserID(ctx context.Context, userID int64) (*jira_credential_dto.Record, bool, error)
	DeleteByUserID(ctx context.Context, userID int64) error
	// FindAll is used by the Admin Dashboard (Chapter 10b) to show which
	// users have a personal credential set, without exposing the token.
	FindAll(ctx context.Context) ([]jira_credential_dto.Record, error)
}
