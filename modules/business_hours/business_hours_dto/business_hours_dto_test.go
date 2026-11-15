package business_hours_dto

import (
	"context"
	"testing"
	"time"
)

func TestHolidayCalendar_ForEmailReturnsItself(t *testing.T) {
	cal := HolidayCalendar{Holidays: map[string]bool{"2026-12-25": true}}
	got := cal.ForEmail(context.Background(), "any@esb.co.id")
	if !got.IsHoliday(time.Date(2026, 12, 25, 10, 0, 0, 0, time.UTC)) {
		t.Fatal("ForEmail must keep the global holidays")
	}
}

func TestHolidayCalendar_WithAddsDatesWithoutMutatingOriginal(t *testing.T) {
	base := HolidayCalendar{Holidays: map[string]bool{"2026-12-25": true}}
	merged := base.With([]string{"2026-11-18"})
	if !merged.Holidays["2026-11-18"] || !merged.Holidays["2026-12-25"] {
		t.Fatalf("merged = %v", merged.Holidays)
	}
	if base.Holidays["2026-11-18"] {
		t.Fatal("With must not mutate the base calendar")
	}
}
