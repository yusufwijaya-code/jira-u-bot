package user_leave_service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/user_leave/user_leave_dto"
)

type fakeRepo struct {
	dates map[string]string // date → note
	reads int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{dates: map[string]string{}} }

func (f *fakeRepo) ListByEmail(context.Context, string) ([]user_leave_dto.Leave, error) {
	var out []user_leave_dto.Leave
	for d, n := range f.dates {
		out = append(out, user_leave_dto.Leave{Date: d, Note: n})
	}
	return out, nil
}

func (f *fakeRepo) Insert(_ context.Context, _ string, dates []string, note string) ([]string, error) {
	var added []string
	for _, d := range dates {
		if _, ok := f.dates[d]; !ok {
			f.dates[d] = note
			added = append(added, d)
		}
	}
	return added, nil
}

func (f *fakeRepo) Delete(_ context.Context, _ string, date string) (bool, error) {
	_, ok := f.dates[date]
	delete(f.dates, date)
	return ok, nil
}

func (f *fakeRepo) DatesByEmail(context.Context, string) ([]string, error) {
	f.reads++
	var out []string
	for d := range f.dates {
		out = append(out, d)
	}
	return out, nil
}

// Friday 2026-11-20 10:00 WIB.
func fixedNow() time.Time { return time.Date(2026, 11, 20, 10, 0, 0, 0, wib) }

func newSvc(repo *fakeRepo, recalc chan string) *userLeaveServiceImpl {
	global := business_hours_dto.HolidayCalendar{Holidays: map[string]bool{"2026-11-24": true}}
	s := NewUserLeaveService(repo, global, func(email string) {
		if recalc != nil {
			recalc <- email
		}
	}).(*userLeaveServiceImpl)
	s.now = fixedNow
	return s
}

func TestAdd_SkipsWeekendAndHolidayAndReportsThem(t *testing.T) {
	s := newSvc(newFakeRepo(), nil)
	// Sat 21 → Tue 24: 21, 22 weekend; 23 working; 24 national holiday.
	res, err := s.Add(context.Background(), "dev@esb.co.id", "2026-11-21", "2026-11-24", "Cuti")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Added) != 1 || res.Added[0] != "2026-11-23" {
		t.Fatalf("Added = %v", res.Added)
	}
	if len(res.Skipped) != 3 {
		t.Fatalf("Skipped = %v", res.Skipped)
	}
	if res.Recalculating {
		t.Fatal("future-only leave must not recalculate")
	}
}

func TestAdd_ExistingDayIsIgnored(t *testing.T) {
	repo := newFakeRepo()
	repo.dates["2026-11-23"] = "old"
	s := newSvc(repo, nil)
	res, err := s.Add(context.Background(), "dev@esb.co.id", "2026-11-23", "", "new")
	if err != nil || len(res.Added) != 0 {
		t.Fatalf("Add = %+v, %v; want nothing added", res, err)
	}
	if repo.dates["2026-11-23"] != "old" {
		t.Fatal("an existing day must stay as it is")
	}
}

func TestAdd_PastDateTriggersRecalc(t *testing.T) {
	recalc := make(chan string, 1)
	s := newSvc(newFakeRepo(), recalc)
	res, err := s.Add(context.Background(), "dev@esb.co.id", "2026-11-18", "", "")
	if err != nil || !res.Recalculating {
		t.Fatalf("Add = %+v, %v", res, err)
	}
	select {
	case got := <-recalc:
		if got != "dev@esb.co.id" {
			t.Fatalf("recalc for %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("recalc hook not called")
	}
}

func TestAdd_Validation(t *testing.T) {
	s := newSvc(newFakeRepo(), nil)
	ctx := context.Background()
	cases := []struct {
		name, from, to, note string
		want                 error
	}{
		{"end before start", "2026-11-25", "2026-11-23", "", ErrInvalidRange},
		{"more than 31 days", "2026-11-23", "2026-12-25", "", ErrInvalidRange},
		{"more than 60 days ago", "2026-09-01", "", "", ErrInvalidRange},
		{"bad format", "bad", "", "", ErrInvalidRange},
		{"note too long", "2026-11-23", "", strings.Repeat("x", 201), ErrInvalidRange},
		{"weekend only", "2026-11-21", "2026-11-22", "", ErrNoWorkingDay},
	}
	for _, c := range cases {
		if _, err := s.Add(ctx, "dev@esb.co.id", c.from, c.to, c.note); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

func TestRemove_UnknownDateIsNotFound(t *testing.T) {
	s := newSvc(newFakeRepo(), nil)
	if _, err := s.Remove(context.Background(), "dev@esb.co.id", "2026-11-18"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestRemove_PastDayTriggersRecalcFutureDayDoesNot(t *testing.T) {
	recalc := make(chan string, 2)
	repo := newFakeRepo()
	repo.dates["2026-11-18"] = ""
	repo.dates["2026-11-25"] = ""
	s := newSvc(repo, recalc)

	if again, err := s.Remove(context.Background(), "dev@esb.co.id", "2026-11-25"); err != nil || again {
		t.Fatalf("future Remove = %v, %v; want no recalc", again, err)
	}
	if again, err := s.Remove(context.Background(), "dev@esb.co.id", "2026-11-18"); err != nil || !again {
		t.Fatalf("past Remove = %v, %v; want recalc", again, err)
	}
	select {
	case <-recalc:
	case <-time.After(time.Second):
		t.Fatal("recalc hook not called")
	}
}

func TestForEmail_MergesLeaveAndCachesUntilChange(t *testing.T) {
	repo := newFakeRepo()
	repo.dates["2026-11-18"] = ""
	s := newSvc(repo, nil)
	ctx := context.Background()

	cal := s.ForEmail(ctx, "dev@esb.co.id")
	if !cal.Holidays["2026-11-18"] || !cal.Holidays["2026-11-24"] {
		t.Fatalf("calendar = %v", cal.Holidays)
	}
	s.ForEmail(ctx, "dev@esb.co.id")
	if repo.reads != 1 {
		t.Fatalf("reads = %d, want cached", repo.reads)
	}
	if _, err := s.Add(ctx, "dev@esb.co.id", "2026-11-23", "", ""); err != nil {
		t.Fatal(err)
	}
	if !s.ForEmail(ctx, "dev@esb.co.id").Holidays["2026-11-23"] {
		t.Fatal("Add must invalidate the cache")
	}
	if s.ForEmail(ctx, "").Holidays["2026-11-18"] {
		t.Fatal("empty email must get the national holidays only")
	}
}
