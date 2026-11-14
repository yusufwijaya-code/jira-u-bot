package jira_client_service

import (
	"context"
	"time"

	"nudge/modules/jira_client/jira_client_dto"
)

type JiraClientService interface {
	SearchCodeProcessIssues(ctx context.Context) ([]jira_client_dto.Issue, error)
	// GetLatestCodeProcessEntryTime returns the timestamp of the MOST RECENT
	// transition into "Code Process" for issueKey — not the first one ever.
	// Used only at poller startup to backfill tickets already mid-flight
	// (spec Business Logic point 7); steady-state polling never calls this.
	GetLatestCodeProcessEntryTime(ctx context.Context, issueKey string) (time.Time, bool, error)
	// LookupUserByEmail reports whether a Jira account matches email and,
	// if so, its accountId — the canonical identifier for matching
	// registered users against issue assignees later (Jira hides
	// emailAddress on assignee objects org-wide, but accountId is always
	// present). accountID is empty when found is false.
	LookupUserByEmail(ctx context.Context, email string) (accountID string, found bool, err error)
	// GetIssueStatus fetches issueKey's CURRENT status directly — used only
	// at poller exit-event time (spec Business Logic point 2), since by
	// then the ticket no longer appears in any "status = Code Process" JQL
	// result and its new status can't be learned any other way.
	GetIssueStatus(ctx context.Context, issueKey string) (string, error)
	// GetTransitions returns the transitions CURRENTLY AVAILABLE from
	// issueKey's present status — callers must search this list for a
	// matching target (To.Name) rather than assuming any particular
	// transition exists, since workflows vary per project/issue type.
	GetTransitions(ctx context.Context, issueKey string) ([]jira_client_dto.Transition, error)
	// TransitionIssue executes transitionID (one of the IDs returned by
	// GetTransitions) against issueKey.
	TransitionIssue(ctx context.Context, issueKey, transitionID string) error
	// SearchAllIssuesByAssignee returns EVERY OMS/CORE issue assigned to
	// accountID, regardless of status — unlike SearchCodeProcessIssues,
	// this is not limited to "Code Process" (Tech Spec "Dashboard Jira
	// Manage", Chapter 9b, My Board). cred == nil uses the central
	// credential; a non-nil cred acts as that user instead (Open Question
	// #2 — only used for a user's own board, never the background poller).
	SearchAllIssuesByAssignee(ctx context.Context, cred *jira_client_dto.Credential, accountID string) ([]jira_client_dto.Issue, error)
	// GetTransitionsAs is GetTransitions, acting as cred instead of the
	// central credential when cred is non-nil (Chapter 9d).
	GetTransitionsAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) ([]jira_client_dto.Transition, error)
	// TransitionIssueAs is TransitionIssue, acting as cred instead of the
	// central credential when cred is non-nil (Chapter 9d) — this is what
	// makes a transition show up in Jira as the real user's own activity.
	TransitionIssueAs(ctx context.Context, cred *jira_client_dto.Credential, issueKey, transitionID string) error
}
