package jira_client_dto

import "regexp"

// IssueRef is the compact issue Jira embeds in parent/subtasks/issuelinks.
type IssueRef struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  Status `json:"status"`
	} `json:"fields"`
}

// IssueLink is one entry of the issuelinks field: exactly one of
// InwardIssue / OutwardIssue is set, seen from the issue that owns it.
type IssueLink struct {
	Type struct {
		Name    string `json:"name"`
		Inward  string `json:"inward"`
		Outward string `json:"outward"`
	} `json:"type"`
	InwardIssue  *IssueRef `json:"inwardIssue,omitempty"`
	OutwardIssue *IssueRef `json:"outwardIssue,omitempty"`
}

// RelationSubtask is the Relation label of a subtask.
const RelationSubtask = "subtask"

// Relation is one child of an issue: a subtask or a linked issue, labelled
// as Jira words it from the parent's side ("relates to", "blocks", "is
// blocked by", ...).
type Relation struct {
	Key   string
	Label string
}

var issueKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-[0-9]+$`)

// IsTrackedProjectKey reports whether issueKey is a well-formed key of a
// project Nudge tracks (CORE/OMS) — anything else never reaches a JQL.
func IsTrackedProjectKey(issueKey string) bool {
	if !issueKeyPattern.MatchString(issueKey) {
		return false
	}
	_, ok := trackingRules[ProjectKeyOf(issueKey)]
	return ok
}

// Relations lists the issue's subtasks then its linked issues (both
// directions), in Jira's order, limited to tracked projects and without
// duplicates or self-links (Tech Spec "Subtask & Linked Item Branches").
func (i Issue) Relations() []Relation {
	seen := map[string]bool{i.Key: true}
	var out []Relation
	add := func(key, label string) {
		if seen[key] || !IsTrackedProjectKey(key) {
			return
		}
		seen[key] = true
		out = append(out, Relation{Key: key, Label: label})
	}
	for _, sub := range i.Fields.Subtasks {
		add(sub.Key, RelationSubtask)
	}
	for _, link := range i.Fields.IssueLinks {
		switch {
		case link.OutwardIssue != nil:
			add(link.OutwardIssue.Key, firstNonEmpty(link.Type.Outward, link.Type.Name))
		case link.InwardIssue != nil:
			add(link.InwardIssue.Key, firstNonEmpty(link.Type.Inward, link.Type.Name))
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
