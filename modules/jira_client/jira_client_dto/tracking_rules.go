package jira_client_dto

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// TrackingRule says which statuses of a project count toward a ticket's
// time budget (Tech Spec "CORE: Waktu Code Review Ikut Dihitung").
type TrackingRule struct {
	// TrackedStatuses: the budget clock runs while the ticket is in any of
	// these; moving between them never stops it.
	TrackedStatuses []string
	// DoneStatus: leaving the tracked statuses for this one is "finished" —
	// the early-completion notice fires (if under budget).
	DoneStatus string
	// ReviewStatus (optional): a tracked status that is the code review.
	ReviewStatus string
}

// trackingRules is the single source of truth for every place that needs
// to know which statuses are tracked: the poller (JQL, exits, session
// start), the reminder engine, and the dashboard. Status names are matched
// case-insensitively (CORE writes "CODE REVIEW", OMS "Code Review").
var trackingRules = map[string]TrackingRule{
	"OMS": {
		TrackedStatuses: []string{StatusCodeProcess},
		DoneStatus:      StatusCodeReview,
	},
	"CORE": {
		TrackedStatuses: []string{StatusCodeProcess, StatusCodeReviewCore},
		DoneStatus:      StatusCodeCompletedCore,
		ReviewStatus:    StatusCodeReviewCore,
	},
}

// defaultTrackingRule applies to any project not in trackingRules (the
// legacy behavior: only Code Process counts, Code Review is "done").
var defaultTrackingRule = trackingRules["OMS"]

// RuleFor returns the tracking rule of a project key ("OMS", "CORE").
func RuleFor(projectKey string) TrackingRule {
	if rule, ok := trackingRules[strings.ToUpper(projectKey)]; ok {
		return rule
	}
	return defaultTrackingRule
}

// RuleForIssue returns the tracking rule of an issue key ("CORE-123").
func RuleForIssue(issueKey string) TrackingRule {
	return RuleFor(ProjectKeyOf(issueKey))
}

// ProjectKeyOf returns "CORE" for "CORE-123" ("" when there's no dash).
func ProjectKeyOf(issueKey string) string {
	if idx := strings.Index(issueKey, "-"); idx > 0 {
		return issueKey[:idx]
	}
	return ""
}

// IsTracked reports whether status counts toward the budget.
func (r TrackingRule) IsTracked(status string) bool {
	for _, tracked := range r.TrackedStatuses {
		if strings.EqualFold(tracked, status) {
			return true
		}
	}
	return false
}

// IsDone reports whether status is the project's "finished" destination.
func (r TrackingRule) IsDone(status string) bool {
	return r.DoneStatus != "" && strings.EqualFold(r.DoneStatus, status)
}

// IsReview reports whether status is the project's tracked review status.
func (r TrackingRule) IsReview(status string) bool {
	return r.ReviewStatus != "" && strings.EqualFold(r.ReviewStatus, status)
}

// TracksReview reports whether this project counts code review time.
func (r TrackingRule) TracksReview() bool {
	return r.ReviewStatus != ""
}

// TrackedIssuesJQL is the poller's search: every ticket of a known project
// in one of that project's tracked statuses. Deterministic (sorted) so it
// is testable and cache-friendly.
func TrackedIssuesJQL() string {
	keys := make([]string, 0, len(trackingRules))
	for key := range trackingRules {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	clauses := make([]string, 0, len(keys))
	for _, key := range keys {
		quoted := make([]string, len(trackingRules[key].TrackedStatuses))
		for i, status := range trackingRules[key].TrackedStatuses {
			quoted[i] = fmt.Sprintf("%q", status)
		}
		clauses = append(clauses, fmt.Sprintf(`(project = %s AND status in (%s))`, key, strings.Join(quoted, ", ")))
	}
	return strings.Join(clauses, " OR ")
}

// TrackedRunTimes walks a changelog against rule: runStart is the latest
// transition from an UNtracked status into a tracked one (the start of the
// current run — CP → CODE REVIEW doesn't restart it), lastEntry the latest
// transition into any tracked status. found=false if the issue never
// entered a tracked status.
func TrackedRunTimes(histories []ChangelogEntry, rule TrackingRule) (runStart, lastEntry time.Time, found bool) {
	for _, history := range histories {
		created, err := time.Parse("2006-01-02T15:04:05.000-0700", history.Created)
		if err != nil {
			continue
		}
		for _, item := range history.Items {
			if item.Field != "status" || !rule.IsTracked(item.ToString) {
				continue
			}
			if !found || created.After(lastEntry) {
				lastEntry = created
			}
			if !rule.IsTracked(item.FromString) && (runStart.IsZero() || created.After(runStart)) {
				runStart = created
			}
			found = true
		}
	}
	if found && runStart.IsZero() {
		// Only tracked→tracked transitions are visible (e.g. created
		// straight into a tracked status): fall back to the last entry.
		runStart = lastEntry
	}
	return runStart, lastEntry, found
}
