package business_hours_dto

import (
	"context"
	"time"
)

type HolidayCalendar struct {
	Holidays map[string]bool
}

// CalendarSource yields the non-working days that apply to one user: the
// national holidays plus, when leave is recorded, that user's leave days
// (Tech Spec "Papan Tiket & Cuti", AC3.1).
type CalendarSource interface {
	ForEmail(ctx context.Context, email string) HolidayCalendar
}

// ForEmail makes a plain calendar a CalendarSource that ignores the user.
func (c HolidayCalendar) ForEmail(context.Context, string) HolidayCalendar { return c }

// With returns a copy of c that also treats dates ("2006-01-02") as holidays.
func (c HolidayCalendar) With(dates []string) HolidayCalendar {
	merged := make(map[string]bool, len(c.Holidays)+len(dates))
	for d, v := range c.Holidays {
		merged[d] = v
	}
	for _, d := range dates {
		merged[d] = true
	}
	return HolidayCalendar{Holidays: merged}
}

func (c HolidayCalendar) IsHoliday(t time.Time) bool {
	if c.Holidays == nil {
		return false
	}
	return c.Holidays[t.Format("2006-01-02")]
}
