package user_leave_dto

// Leave is one full day of leave (Tech Spec "Papan Tiket & Cuti").
type Leave struct {
	Date string `json:"date"` // 2006-01-02
	Note string `json:"note"`
}

// AddResult reports what a range request stored.
type AddResult struct {
	Added []string `json:"added"`
	// Skipped are the weekends and national holidays inside the range.
	Skipped []string `json:"skipped"`
	// Recalculating is true when a past day changed and tracked time is
	// being rebuilt from Jira in the background (AC4.2).
	Recalculating bool `json:"recalculating"`
}
