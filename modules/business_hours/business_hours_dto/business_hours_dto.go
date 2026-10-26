package business_hours_dto

import "time"

type HolidayCalendar struct {
	Holidays map[string]bool
}

func (c HolidayCalendar) IsHoliday(t time.Time) bool {
	if c.Holidays == nil {
		return false
	}
	return c.Holidays[t.Format("2006-01-02")]
}
