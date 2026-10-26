package business_hours_service

import (
	"testing"
	"time"

	"jira-u-bot/modules/business_hours/business_hours_dto"
)

func mustLoadLocation(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("failed to load Asia/Jakarta location: %v", err)
	}
	return loc
}

func TestElapsedBusinessSeconds_SameDayWithinWorkHours(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	start := time.Date(2026, 10, 26, 9, 0, 0, 0, loc) // Monday 09:00
	end := time.Date(2026, 10, 26, 10, 0, 0, 0, loc)  // Monday 10:00

	got := svc.ElapsedBusinessSeconds(start, end, cal)
	want := int64(3600)

	if got != want {
		t.Errorf("got %d seconds, want %d", got, want)
	}
}

func TestElapsedBusinessSeconds_ExcludesLunchBreak(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	start := time.Date(2026, 10, 26, 11, 30, 0, 0, loc) // Monday 11:30
	end := time.Date(2026, 10, 26, 13, 30, 0, 0, loc)   // Monday 13:30

	got := svc.ElapsedBusinessSeconds(start, end, cal)
	// 11:30-12:00 (30m, before lunch) + 13:00-13:30 (30m, after lunch) = 1h. The
	// 12:00-13:00 lunch hour itself contributes zero.
	want := int64(3600)

	if got != want {
		t.Errorf("got %d seconds, want %d", got, want)
	}
}

func TestElapsedBusinessSeconds_BeforeWorkStartClampsToZeroForThatSegment(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	start := time.Date(2026, 10, 26, 7, 0, 0, 0, loc) // Monday 07:00, before 08:30 open
	end := time.Date(2026, 10, 26, 9, 0, 0, 0, loc)   // Monday 09:00

	got := svc.ElapsedBusinessSeconds(start, end, cal)
	// Only 08:30-09:00 counts (30 minutes); 07:00-08:30 is outside work hours.
	want := int64(1800)

	if got != want {
		t.Errorf("got %d seconds, want %d", got, want)
	}
}

func TestElapsedBusinessSeconds_AfterWorkEndClampsToZeroForThatSegment(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	start := time.Date(2026, 10, 26, 17, 0, 0, 0, loc) // Monday 17:00
	end := time.Date(2026, 10, 26, 19, 0, 0, 0, loc)   // Monday 19:00, after 17:30 close

	got := svc.ElapsedBusinessSeconds(start, end, cal)
	// Only 17:00-17:30 counts (30 minutes); 17:30-19:00 is outside work hours.
	want := int64(1800)

	if got != want {
		t.Errorf("got %d seconds, want %d", got, want)
	}
}

func TestElapsedBusinessSeconds_ExcludesWeekend(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	start := time.Date(2026, 10, 23, 17, 0, 0, 0, loc) // Friday 17:00
	end := time.Date(2026, 10, 26, 8, 30, 0, 0, loc)   // Monday 08:30 (exact work start)

	got := svc.ElapsedBusinessSeconds(start, end, cal)
	// Friday 17:00-17:30 (30m) is the only contribution. Sat/Sun contribute 0.
	// Monday 00:00-08:30 is all pre-work-hours, so it also contributes 0 —
	// the window ends exactly at 08:30, before any billable second on Monday.
	want := int64(1800)

	if got != want {
		t.Errorf("got %d seconds, want %d", got, want)
	}
}

func TestElapsedBusinessSeconds_ExcludesConfiguredHoliday(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}
	cal.Holidays = map[string]bool{"2026-10-27": true} // Tuesday, marked as a holiday

	start := time.Date(2026, 10, 27, 8, 30, 0, 0, loc)
	end := time.Date(2026, 10, 27, 17, 30, 0, 0, loc)

	got := svc.ElapsedBusinessSeconds(start, end, cal)
	want := int64(0)

	if got != want {
		t.Errorf("got %d seconds, want %d (full work day is a configured holiday)", got, want)
	}
}

func TestElapsedBusinessSeconds_SpansMultipleFullWorkDays(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	start := time.Date(2026, 10, 26, 8, 30, 0, 0, loc) // Monday, work start
	end := time.Date(2026, 10, 28, 8, 30, 0, 0, loc)   // Wednesday, same time

	got := svc.ElapsedBusinessSeconds(start, end, cal)
	// Monday full day (8h) + Tuesday full day (8h) + 0 on Wednesday
	// (the window ends exactly at Wednesday's work start, before any billable second).
	want := int64(8 * 3600 * 2)

	if got != want {
		t.Errorf("got %d seconds, want %d", got, want)
	}
}

func TestElapsedBusinessSeconds_EndBeforeStartReturnsZero(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	start := time.Date(2026, 10, 26, 10, 0, 0, 0, loc)
	end := time.Date(2026, 10, 26, 9, 0, 0, 0, loc)

	got := svc.ElapsedBusinessSeconds(start, end, cal)
	want := int64(0)

	if got != want {
		t.Errorf("got %d seconds, want %d for an inverted (end before start) range", got, want)
	}
}

func TestElapsedBusinessSeconds_NormalizesUTCInputToJakartaBeforeComputing(t *testing.T) {
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	// 2026-10-26 is a Monday. 09:00 WIB == 02:00 UTC. A naive UTC-located
	// calculation would see 02:00-02:05 UTC, entirely before the 08:30 UTC
	// work-day window, and wrongly compute 0 elapsed seconds.
	start := time.Date(2026, 10, 26, 2, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 26, 2, 5, 0, 0, time.UTC)

	got := svc.ElapsedBusinessSeconds(start, end, cal)
	want := int64(300)

	if got != want {
		t.Errorf("got %d seconds, want %d — UTC-located input (matching what the sqlite driver returns for CURRENT_TIMESTAMP) must be normalized to Asia/Jakarta before the business-hours window is computed", got, want)
	}
}

func TestDeadlineAfterBusinessSeconds_SameDayWithinWorkHours(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	from := time.Date(2026, 10, 26, 9, 0, 0, 0, loc) // Monday 09:00
	got := svc.DeadlineAfterBusinessSeconds(from, 3600, cal)
	want := time.Date(2026, 10, 26, 10, 0, 0, 0, loc)

	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDeadlineAfterBusinessSeconds_SpansLunchBreak(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	from := time.Date(2026, 10, 26, 11, 30, 0, 0, loc) // Monday 11:30
	got := svc.DeadlineAfterBusinessSeconds(from, 3600, cal)
	// 11:30-12:00 (30m) before lunch + 13:00-13:30 (30m) after lunch = 1h total.
	want := time.Date(2026, 10, 26, 13, 30, 0, 0, loc)

	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDeadlineAfterBusinessSeconds_FromBeforeWorkStartClampsToOpen(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	from := time.Date(2026, 10, 26, 7, 0, 0, 0, loc) // Monday 07:00, before 08:30 open
	got := svc.DeadlineAfterBusinessSeconds(from, 1800, cal)
	want := time.Date(2026, 10, 26, 9, 0, 0, 0, loc) // 08:30 open + 30 min

	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDeadlineAfterBusinessSeconds_FromAfterWorkEndRollsToNextDay(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	from := time.Date(2026, 10, 26, 19, 0, 0, 0, loc) // Monday 19:00, after 17:30 close
	got := svc.DeadlineAfterBusinessSeconds(from, 1800, cal)
	want := time.Date(2026, 10, 27, 9, 0, 0, 0, loc) // Tuesday 08:30 open + 30 min

	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDeadlineAfterBusinessSeconds_SkipsWeekend(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	from := time.Date(2026, 10, 23, 17, 0, 0, 0, loc) // Friday 17:00
	got := svc.DeadlineAfterBusinessSeconds(from, 3600, cal)
	// Friday 17:00-17:30 (30m) + Sat/Sun skipped + Monday 08:30 open + 30 min.
	want := time.Date(2026, 10, 26, 9, 0, 0, 0, loc)

	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDeadlineAfterBusinessSeconds_SkipsConfiguredHoliday(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{Holidays: map[string]bool{"2026-10-27": true}} // Tuesday is a holiday

	from := time.Date(2026, 10, 26, 17, 0, 0, 0, loc) // Monday 17:00
	got := svc.DeadlineAfterBusinessSeconds(from, 3600, cal)
	// Monday 17:00-17:30 (30m) + Tuesday is a holiday, skipped + Wednesday 08:30 open + 30 min.
	want := time.Date(2026, 10, 28, 9, 0, 0, 0, loc)

	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDeadlineAfterBusinessSeconds_ZeroOrNegativeRemainingReturnsFrom(t *testing.T) {
	loc := mustLoadLocation(t)
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	from := time.Date(2026, 10, 26, 9, 0, 0, 0, loc)

	if got := svc.DeadlineAfterBusinessSeconds(from, 0, cal); !got.Equal(from) {
		t.Errorf("remainingSeconds=0: got %v, want %v (the from instant, unchanged)", got, from)
	}
	if got := svc.DeadlineAfterBusinessSeconds(from, -100, cal); !got.Equal(from) {
		t.Errorf("remainingSeconds=-100: got %v, want %v (the from instant, unchanged)", got, from)
	}
}

func TestDeadlineAfterBusinessSeconds_NormalizesUTCInputToJakarta(t *testing.T) {
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{}

	// 02:00 UTC == 09:00 WIB (Monday). 1 hour of business time should land
	// at 10:00 WIB == 03:00 UTC the same day.
	from := time.Date(2026, 10, 26, 2, 0, 0, 0, time.UTC)
	got := svc.DeadlineAfterBusinessSeconds(from, 3600, cal)

	loc := mustLoadLocation(t)
	want := time.Date(2026, 10, 26, 10, 0, 0, 0, loc)

	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLoadHolidayCalendar_ReadsDatesFromJSONFile(t *testing.T) {
	svc := NewBusinessHoursService()

	cal, err := svc.LoadHolidayCalendar("testdata/holidays_sample.json")
	if err != nil {
		t.Fatalf("LoadHolidayCalendar returned error: %v", err)
	}

	if !cal.IsHoliday(time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)) {
		t.Error("expected 2026-01-01 to be a holiday per testdata/holidays_sample.json")
	}
	if cal.IsHoliday(time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)) {
		t.Error("2026-01-02 should not be a holiday")
	}
}
