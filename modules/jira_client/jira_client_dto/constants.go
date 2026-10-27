package jira_client_dto

// StatusCodeProcess is the single source of truth for the Jira status name
// this whole service watches for — spec Note C2 (shared contract constant,
// avoids hardcoding "Code Process" as a string literal in multiple files).
const StatusCodeProcess = "Code Process"

// StoryPointFieldID is the Jira custom field ID for "Story point estimate",
// verified against the real esb-id.atlassian.net site during spec research.
const StoryPointFieldID = "customfield_10016"

// StoryPointFieldIDCore is the Jira custom field ID the CORE project uses
// for "Story Points" — confirmed live that OMS and CORE use two entirely
// different custom fields for the same concept: a CORE ticket's
// customfield_10016 is always nil, and an OMS ticket's customfield_11053
// is always nil. Both must be requested and coalesced (IssueFields.
// EffectiveStoryPoints) to support both projects.
const StoryPointFieldIDCore = "customfield_11053"

// StatusCodeReview is the destination status Early Completion Notice
// checks for (spec AC4.1/AC4.5) — a ticket leaving "Code Process" to any
// OTHER status never triggers this notice.
const StatusCodeReview = "Code Review"
