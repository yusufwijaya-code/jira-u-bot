package business_hours_service

import (
	"time"

	"jira-u-bot/modules/business_hours/business_hours_dto"
)

type BusinessHoursService interface {
	ElapsedBusinessSeconds(start, end time.Time, cal business_hours_dto.HolidayCalendar) int64
	// DeadlineAfterBusinessSeconds is the inverse of ElapsedBusinessSeconds:
	// given a starting instant and a remaining business-seconds budget, it
	// returns the future wall-clock instant at which that many business
	// seconds will have elapsed — used to show the real clock time a ticket
	// must be finished by, not just a countdown.
	DeadlineAfterBusinessSeconds(from time.Time, remainingSeconds int64, cal business_hours_dto.HolidayCalendar) time.Time
	// IsWithinBusinessHours reports whether t itself falls inside a working
	// segment (weekday, not a holiday, not lunch break) — used to gate
	// things that must pause outside office hours rather than just
	// measuring elapsed time within them.
	IsWithinBusinessHours(t time.Time, cal business_hours_dto.HolidayCalendar) bool
	LoadHolidayCalendar(path string) (business_hours_dto.HolidayCalendar, error)
}
