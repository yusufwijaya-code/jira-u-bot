package chat_app_dto

// The subset of the Google Chat cardsV2 schema Nudge builds (phase 3).

type CardWithID struct {
	CardID string `json:"cardId"`
	Card   Card   `json:"card"`
}

type Card struct {
	Header   *CardHeader `json:"header,omitempty"`
	Sections []Section   `json:"sections"`
}

type CardHeader struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
}

type Section struct {
	Widgets []Widget `json:"widgets"`
}

type Widget struct {
	TextParagraph *TextParagraph `json:"textParagraph,omitempty"`
	ButtonList    *ButtonList    `json:"buttonList,omitempty"`
}

type TextParagraph struct {
	Text string `json:"text"`
}

type ButtonList struct {
	Buttons []Button `json:"buttons"`
}

type Button struct {
	Text    string  `json:"text"`
	OnClick OnClick `json:"onClick"`
}

type OnClick struct {
	Action   *OnClickAction `json:"action,omitempty"`
	OpenLink *OpenLink      `json:"openLink,omitempty"`
}

type OnClickAction struct {
	Function   string            `json:"function"`
	Parameters []ActionParameter `json:"parameters,omitempty"`
}

type OpenLink struct {
	URL string `json:"url"`
}
