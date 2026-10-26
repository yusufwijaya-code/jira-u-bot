package business_hours_service

import (
	"encoding/json"
	"os"
	"time"

	"jira-u-bot/modules/business_hours/business_hours_dto"
)

const (
	workStartHour   = 8
	workStartMinute = 30
	workEndHour     = 17
	workEndMinute   = 30
	lunchStartHour  = 12
	lunchEndHour    = 13
)

type businessHoursServiceImpl struct{}

func NewBusinessHoursService() BusinessHoursService {
	return &businessHoursServiceImpl{}
}

// ElapsedBusinessSeconds walks day by day from start to end, summing only the
// seconds that fall inside business hours on working days. It is a pure
// function of (start, end, cal) — no clock reads, no I/O — so it is cheap to
// call on every 30-second poll cycle (spec Component Poller, Business Logic
// point 5) without caching.
func (s *businessHoursServiceImpl) ElapsedBusinessSeconds(start, end time.Time, cal business_hours_dto.HolidayCalendar) int64 {
	start = start.In(jakartaLocation)
	end = end.In(jakartaLocation)

	if !end.After(start) {
		return 0
	}

	var total int64
	cursor := start

	for cursor.Before(end) {
		dayEnd := time.Date(cursor.Year(), cursor.Month(), cursor.Day(), 23, 59, 59, 0, cursor.Location())
		segmentEnd := end
		if dayEnd.Before(segmentEnd) {
			segmentEnd = dayEnd
		}

		total += businessSecondsInDay(cursor, segmentEnd, cal)

		cursor = time.Date(cursor.Year(), cursor.Month(), cursor.Day(), 0, 0, 0, 0, cursor.Location()).AddDate(0, 0, 1)
	}

	return total
}

func businessSecondsInDay(dayStart, dayEnd time.Time, cal business_hours_dto.HolidayCalendar) int64 {
	weekday := dayStart.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return 0
	}
	if cal.IsHoliday(dayStart) {
		return 0
	}

	y, m, d := dayStart.Date()
	loc := dayStart.Location()

	morningStart := time.Date(y, m, d, workStartHour, workStartMinute, 0, 0, loc)
	morningEnd := time.Date(y, m, d, lunchStartHour, 0, 0, 0, loc)
	afternoonStart := time.Date(y, m, d, lunchEndHour, 0, 0, 0, loc)
	afternoonEnd := time.Date(y, m, d, workEndHour, workEndMinute, 0, 0, loc)

	var seconds int64
	seconds += overlapSeconds(dayStart, dayEnd, morningStart, morningEnd)
	seconds += overlapSeconds(dayStart, dayEnd, afternoonStart, afternoonEnd)

	return seconds
}

// overlapSeconds returns the length, in seconds, of the intersection of
// [aStart, aEnd) and [bStart, bEnd). Returns 0 if they don't overlap.
func overlapSeconds(aStart, aEnd, bStart, bEnd time.Time) int64 {
	start := aStart
	if bStart.After(start) {
		start = bStart
	}
	end := aEnd
	if bEnd.Before(end) {
		end = bEnd
	}
	if end.Before(start) {
		return 0
	}
	return int64(end.Sub(start).Seconds())
}

// LoadHolidayCalendar reads a plain JSON array of "YYYY-MM-DD" strings — the
// vendored, simplified format described in spec Note C4. It is NOT a live
// call to any holiday API; the file at path is refreshed manually once a
// year (see Task 1b.2).
func (s *businessHoursServiceImpl) LoadHolidayCalendar(path string) (business_hours_dto.HolidayCalendar, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return business_hours_dto.HolidayCalendar{}, err
	}

	var dates []string
	if err := json.Unmarshal(raw, &dates); err != nil {
		return business_hours_dto.HolidayCalendar{}, err
	}

	holidays := make(map[string]bool, len(dates))
	for _, d := range dates {
		holidays[d] = true
	}

	return business_hours_dto.HolidayCalendar{Holidays: holidays}, nil
}
