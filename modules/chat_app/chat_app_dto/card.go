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
	Header string `json:"header,omitempty"`
	// Collapsible sections show only the first UncollapsibleWidgetsCount
	// widgets until the user expands them.
	Collapsible               bool     `json:"collapsible,omitempty"`
	UncollapsibleWidgetsCount int      `json:"uncollapsibleWidgetsCount,omitempty"`
	Widgets                   []Widget `json:"widgets"`
}

type Widget struct {
	TextParagraph  *TextParagraph  `json:"textParagraph,omitempty"`
	DecoratedText  *DecoratedText  `json:"decoratedText,omitempty"`
	ButtonList     *ButtonList     `json:"buttonList,omitempty"`
	DateTimePicker *DateTimePicker `json:"dateTimePicker,omitempty"`
	TextInput      *TextInput      `json:"textInput,omitempty"`
	Divider        *struct{}       `json:"divider,omitempty"`
}

type TextParagraph struct {
	Text string `json:"text"`
}

// DecoratedText is one row: a small label on top, the main text, a small
// label below and an optional button on the right.
type DecoratedText struct {
	TopLabel    string  `json:"topLabel,omitempty"`
	Text        string  `json:"text"`
	BottomLabel string  `json:"bottomLabel,omitempty"`
	WrapText    bool    `json:"wrapText,omitempty"`
	Button      *Button `json:"button,omitempty"`
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

// DateTimePicker types.
const PickerDateOnly = "DATE_ONLY"

// DateTimePicker is a date field; its value arrives in the click event's
// common.formInputs under Name.
type DateTimePicker struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Type  string `json:"type"`
	// ValueMsEpoch preselects a date (UTC midnight of that day), 0 = empty.
	ValueMsEpoch int64 `json:"valueMsEpoch,omitempty,string"`
}

// TextInput is a one-line text field (value in common.formInputs).
type TextInput struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	HintText string `json:"hintText,omitempty"`
}
