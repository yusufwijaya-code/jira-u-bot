package jira_client_dto

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// TrackingRule says which statuses of a project count toward a ticket's
// time budget and where its Story Points live (Tech Spec "Aturan Board").
// Rules come from ms_tracking_rule through SetRules.
type TrackingRule struct {
	// ProjectKey: the Jira project ("CORE"); "" for the fallback rule.
	ProjectKey string
	// TrackedStatuses: the budget clock runs while the ticket is in any of
	// these; moving between them never stops it.
	TrackedStatuses []string
	// DoneStatus: leaving the tracked statuses for this one is "finished" —
	// the early-completion notice fires (if under budget).
	DoneStatus string
	// ReviewStatus (optional): a tracked status that is the code review.
	ReviewStatus string
	// StoryPointFields: custom field IDs holding Story Points, in read
	// priority (the first non-null one wins).
	StoryPointFields []string
	// StoryPointWriteField / StoryPointWriteFieldSubtask: where Buat Card
	// writes Story Points for a normal card / a subtask.
	StoryPointWriteField        string
	StoryPointWriteFieldSubtask string
	// CreateFields: per issue type id, the OPTIONAL createmeta fields the
	// admin chose to show on the Buat Card form (required ones always show).
	CreateFields map[string][]string
}

// ExtraCreateField reports whether the admin chose fieldID for typeID.
func (r TrackingRule) ExtraCreateField(typeID, fieldID string) bool {
	for _, id := range r.CreateFields[typeID] {
		if id == fieldID {
			return true
		}
	}
	return false
}

// ruleSet is one immutable snapshot of the active rules: readers load it
// once (lock-free) and never see a half-swapped registry.
type ruleSet struct {
	byKey map[string]TrackingRule
	keys  []string // sorted
}

var currentRules atomic.Pointer[ruleSet]

func init() { SetRules(BuiltinRules()) }

// BuiltinRules are OMS & CORE exactly as the 0021b seed — the registry's
// content until tracking_rule loads the table (and in unit tests).
func BuiltinRules() []TrackingRule {
	return []TrackingRule{
		{
			ProjectKey:                  "CORE",
			TrackedStatuses:             []string{StatusCodeProcess, StatusCodeReviewCore},
			DoneStatus:                  StatusCodeCompletedCore,
			ReviewStatus:                StatusCodeReviewCore,
			StoryPointFields:            []string{StoryPointFieldIDCoreEstimate, StoryPointFieldIDCore},
			StoryPointWriteField:        StoryPointFieldIDCoreEstimate,
			StoryPointWriteFieldSubtask: StoryPointFieldIDCore,
		},
		{
			ProjectKey:                  "OMS",
			TrackedStatuses:             []string{StatusCodeProcess},
			DoneStatus:                  StatusCodeReview,
			StoryPointFields:            []string{StoryPointFieldID},
			StoryPointWriteField:        StoryPointFieldID,
			StoryPointWriteFieldSubtask: StoryPointFieldID,
		},
	}
}

// SetRules installs the ACTIVE rules (every rule passed counts as active).
// Keys are upper-cased; a rule without a project key or tracked status is
// skipped. An empty list tracks nothing: TrackedIssuesJQL returns "".
func SetRules(active []TrackingRule) {
	set := &ruleSet{byKey: make(map[string]TrackingRule, len(active))}
	for _, r := range active {
		key := strings.ToUpper(strings.TrimSpace(r.ProjectKey))
		if key == "" || len(r.TrackedStatuses) == 0 {
			continue
		}
		r.ProjectKey = key
		r.TrackedStatuses = append([]string(nil), r.TrackedStatuses...)
		r.StoryPointFields = append([]string(nil), r.StoryPointFields...)
		r.CreateFields = copyCreateFields(r.CreateFields)
		if _, dup := set.byKey[key]; !dup {
			set.keys = append(set.keys, key)
		}
		set.byKey[key] = r
	}
	sort.Strings(set.keys)
	currentRules.Store(set)
}

func copyCreateFields(m map[string][]string) map[string][]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string][]string, len(m))
	for k, v := range m {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func snapshot() *ruleSet { return currentRules.Load() }

// defaultTrackingRule applies to a project without an active rule (the
// legacy behavior: only Code Process counts, Code Review is "done").
var defaultTrackingRule = TrackingRule{TrackedStatuses: []string{StatusCodeProcess}, DoneStatus: StatusCodeReview}

// RuleFor returns the tracking rule of a project key ("OMS", "CORE").
func RuleFor(projectKey string) TrackingRule {
	if rule, ok := snapshot().byKey[strings.ToUpper(projectKey)]; ok {
		return rule
	}
	return defaultTrackingRule
}

// RuleForIssue returns the tracking rule of an issue key ("CORE-123").
func RuleForIssue(issueKey string) TrackingRule {
	return RuleFor(ProjectKeyOf(issueKey))
}

// IsTrackedProject reports whether projectKey has an active rule.
func IsTrackedProject(projectKey string) bool {
	_, ok := snapshot().byKey[strings.ToUpper(projectKey)]
	return ok
}

// ActiveProjectKeys returns the projects with an active rule, sorted.
func ActiveProjectKeys() []string {
	return append([]string(nil), snapshot().keys...)
}

// StoryPointFieldIDs is every active rule's Story Point fields (project
// order, then priority), without duplicates — the fields= of issue searches.
func StoryPointFieldIDs() []string {
	set := snapshot()
	seen := map[string]bool{}
	var out []string
	for _, key := range set.keys {
		for _, id := range set.byKey[key].StoryPointFields {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

// ProjectsJQLIn is "project in (CORE, OMS)" over the active projects, or ""
// when none is active (callers then skip the search).
func ProjectsJQLIn() string {
	keys := snapshot().keys
	if len(keys) == 0 {
		return ""
	}
	return "project in (" + strings.Join(keys, ", ") + ")"
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

// TrackedIssuesJQL is the poller's search: every ticket of an active project
// in one of that project's tracked statuses; "" when no rule is active.
// Deterministic (sorted) so it is testable and cache-friendly.
func TrackedIssuesJQL() string {
	set := snapshot()
	clauses := make([]string, 0, len(set.keys))
	for _, key := range set.keys {
		rule := set.byKey[key]
		quoted := make([]string, len(rule.TrackedStatuses))
		for i, status := range rule.TrackedStatuses {
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

// TrackedSeconds sums the time a ticket spent in rule's tracked statuses
// across EVERY run in its changelog (Tech Spec "Active Sprint Tracking &
// Changelog Backfill", AC3.3); elapsed measures one run (business hours).
// A run still open at the end of the changelog is NOT counted — a ticket
// currently in a tracked status belongs to the poller. found=false if the
// ticket never entered a tracked status.
func TrackedSeconds(histories []ChangelogEntry, rule TrackingRule, elapsed func(start, end time.Time) int64) (seconds int64, found bool) {
	type change struct {
		at       time.Time
		from, to string
	}
	var changes []change
	for _, history := range histories {
		created, err := time.Parse("2006-01-02T15:04:05.000-0700", history.Created)
		if err != nil {
			continue
		}
		for _, item := range history.Items {
			if item.Field == "status" {
				changes = append(changes, change{created, item.FromString, item.ToString})
			}
		}
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].at.Before(changes[j].at) })

	var runStart *time.Time
	for _, c := range changes {
		switch {
		case runStart == nil && rule.IsTracked(c.to):
			at := c.at
			runStart = &at
			found = true
		case runStart != nil && !rule.IsTracked(c.to):
			seconds += elapsed(*runStart, c.at)
			runStart = nil
		}
	}
	return seconds, found
}
