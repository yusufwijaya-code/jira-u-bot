package chat_interactive_dto

// ChatEvent is the body Google Chat POSTs to our interactive bot endpoint
// when a user @mentions the app or invokes a slash command. Field names
// follow Google's documented event shape (Tech Spec Open Question #1) —
// not yet live-verified against a real payload; adjust if a live event
// turns out to differ, per the Tech Spec's Deployment Checklist.
type ChatEvent struct {
	Type              string             `json:"type"`
	User              ChatEventUser      `json:"user"`
	AppCommandPayload *AppCommandPayload `json:"appCommandPayload"`
}

type ChatEventUser struct {
	Email string `json:"email"`
}

type AppCommandPayload struct {
	AppCommandMetadata AppCommandMetadata `json:"appCommandMetadata"`
	ArgumentText       string             `json:"argumentText"`
}

type AppCommandMetadata struct {
	AppCommandID string `json:"appCommandId"`
}
