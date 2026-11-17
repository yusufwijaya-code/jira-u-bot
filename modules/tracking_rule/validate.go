package tracking_rule

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var projectKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,19}$`)

// NormalizeProjectKey trims and upper-cases raw (AC3.8); ok=false when the
// result is not a valid Jira project key.
func NormalizeProjectKey(raw string) (string, bool) {
	key := strings.ToUpper(strings.TrimSpace(raw))
	return key, projectKeyPattern.MatchString(key)
}

func containsFold(list []string, value string) bool {
	for _, v := range list {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}

// buildRule validates in against the project's Jira data (AC3.7) and returns
// the rule with Jira's spelling of every status; errs holds one message per
// invalid input (empty = valid).
func buildRule(in RuleInput, l Lookup) (Rule, map[string]string) {
	errs := map[string]string{}
	jiraStatus := func(name string) (string, bool) {
		for _, s := range l.Statuses {
			if strings.EqualFold(s, strings.TrimSpace(name)) {
				return s, true
			}
		}
		return "", false
	}
	notInJira := func(name string) string {
		return fmt.Sprintf("Status %q tidak ada di workflow Jira project ini", strings.TrimSpace(name))
	}

	var rule Rule
	for _, raw := range in.TrackedStatuses {
		status, ok := jiraStatus(raw)
		if !ok {
			errs["trackedStatuses"] = notInJira(raw)
			continue
		}
		if !containsFold(rule.TrackedStatuses, status) {
			rule.TrackedStatuses = append(rule.TrackedStatuses, status)
		}
	}
	if len(in.TrackedStatuses) == 0 {
		errs["trackedStatuses"] = "Pilih minimal satu status yang dihitung"
	}

	switch done, ok := jiraStatus(in.DoneStatus); {
	case strings.TrimSpace(in.DoneStatus) == "":
		errs["doneStatus"] = "Status selesai wajib dipilih"
	case !ok:
		errs["doneStatus"] = notInJira(in.DoneStatus)
	case containsFold(rule.TrackedStatuses, done):
		errs["doneStatus"] = "Status selesai tidak boleh termasuk status yang dihitung"
	default:
		rule.DoneStatus = done
	}

	if in.ReviewStatus != nil && strings.TrimSpace(*in.ReviewStatus) != "" {
		review, ok := jiraStatus(*in.ReviewStatus)
		if !ok || !containsFold(rule.TrackedStatuses, review) {
			errs["reviewStatus"] = "Status review harus salah satu status yang dihitung"
		} else {
			rule.ReviewStatus = review
		}
	}

	numeric := map[string]bool{}
	for _, f := range l.NumberFields {
		numeric[f.ID] = true
	}
	for _, raw := range in.StoryPointFields {
		id := strings.TrimSpace(raw)
		switch {
		case !numeric[id]:
			errs["storyPointFields"] = fmt.Sprintf("Field %s tidak ada di Jira atau bukan field angka", id)
		case slices.Contains(rule.StoryPointFields, id):
			errs["storyPointFields"] = fmt.Sprintf("Field %s dipilih lebih dari sekali", id)
		default:
			rule.StoryPointFields = append(rule.StoryPointFields, id)
		}
	}
	if len(in.StoryPointFields) == 0 {
		errs["storyPointFields"] = "Pilih minimal satu field Story Point"
	}

	writeField := func(name, raw, label string) string {
		value := strings.TrimSpace(raw)
		switch {
		case value == "":
			errs[name] = "Field SP tulis (" + label + ") wajib dipilih"
		case !slices.Contains(rule.StoryPointFields, value):
			errs[name] = "Field SP tulis (" + label + ") harus salah satu field SP baca"
		}
		return value
	}
	rule.StoryPointWriteField = writeField("storyPointWriteField", in.StoryPointWriteField, "card biasa")
	rule.StoryPointWriteFieldSubtask = writeField("storyPointWriteFieldSubtask", in.StoryPointWriteFieldSubtask, "subtask")
	return rule, errs
}
