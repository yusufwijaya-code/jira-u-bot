package jira_client_dto

// StatusCodeProcess is the single source of truth for the Jira status name
// this whole service watches for — spec Note C2 (shared contract constant,
// avoids hardcoding "Code Process" as a string literal in multiple files).
const StatusCodeProcess = "Code Process"

// StoryPointFieldID is the Jira custom field ID for "Story point estimate",
// verified against the real esb-id.atlassian.net site during spec research.
const StoryPointFieldID = "customfield_10016"
