package user_leave_service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
	"unicode/utf8"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/user_leave/user_leave_dto"
	"nudge/modules/user_leave/user_leave_repository"
)

const (
	dateLayout   = "2006-01-02"
	maxRangeDays = 31
	maxPastDays  = 60
	maxNoteRunes = 200
	cacheTTL     = 5 * time.Minute
)

var wib = time.FixedZone("WIB", 7*3600)

type cachedCalendar struct {
	cal     business_hours_dto.HolidayCalendar
	expires time.Time
}

type userLeaveServiceImpl struct {
	repo         user_leave_repository.UserLeaveRepository
	global       business_hours_dto.HolidayCalendar
	onPastChange func(email string)
	now          func() time.Time

	mu    sync.Mutex
	cache map[string]cachedCalendar
}

// NewUserLeaveService: global is the national holiday calendar.
// onPastChange (optional) runs in its own goroutine whenever a change
// touches today or an earlier day, so tracked time can be rebuilt (AC4.2).
func NewUserLeaveService(repo user_leave_repository.UserLeaveRepository, global business_hours_dto.HolidayCalendar, onPastChange func(email string)) UserLeaveService {
	return &userLeaveServiceImpl{
		repo: repo, global: global, onPastChange: onPastChange,
		now: time.Now, cache: map[string]cachedCalendar{},
	}
}

func (s *userLeaveServiceImpl) today() time.Time {
	n := s.now().In(wib)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, wib)
}

func (s *userLeaveServiceImpl) List(ctx context.Context, email string) ([]user_leave_dto.Leave, error) {
	return s.repo.ListByEmail(ctx, email)
}

func (s *userLeaveServiceImpl) Add(ctx context.Context, email, from, to, note string) (user_leave_dto.AddResult, error) {
	if to == "" {
		to = from
	}
	start, err1 := time.ParseInLocation(dateLayout, from, wib)
	end, err2 := time.ParseInLocation(dateLayout, to, wib)
	if err1 != nil || err2 != nil {
		return user_leave_dto.AddResult{}, fmt.Errorf("%w: date format", ErrInvalidRange)
	}
	today := s.today()
	switch {
	case end.Before(start):
		return user_leave_dto.AddResult{}, fmt.Errorf("%w: end before start", ErrInvalidRange)
	case int(end.Sub(start).Hours()/24)+1 > maxRangeDays:
		return user_leave_dto.AddResult{}, fmt.Errorf("%w: more than %d days", ErrInvalidRange, maxRangeDays)
	case start.Before(today.AddDate(0, 0, -maxPastDays)):
		return user_leave_dto.AddResult{}, fmt.Errorf("%w: more than %d days ago", ErrInvalidRange, maxPastDays)
	case utf8.RuneCountInString(note) > maxNoteRunes:
		return user_leave_dto.AddResult{}, fmt.Errorf("%w: note too long", ErrInvalidRange)
	}

	res := user_leave_dto.AddResult{Added: []string{}, Skipped: []string{}}
	var working []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday || s.global.IsHoliday(d) {
			res.Skipped = append(res.Skipped, d.Format(dateLayout))
			continue
		}
		working = append(working, d.Format(dateLayout))
	}
	if len(working) == 0 {
		return user_leave_dto.AddResult{}, ErrNoWorkingDay
	}

	added, err := s.repo.Insert(ctx, email, working, note)
	if err != nil {
		return user_leave_dto.AddResult{}, err
	}
	if added != nil {
		res.Added = added
	}
	s.invalidate(email)
	if len(added) > 0 && !start.After(today) {
		res.Recalculating = s.triggerRecalc(email)
	}
	return res, nil
}

func (s *userLeaveServiceImpl) Remove(ctx context.Context, email, date string) (bool, error) {
	day, err := time.ParseInLocation(dateLayout, date, wib)
	if err != nil {
		return false, fmt.Errorf("%w: date format", ErrInvalidRange)
	}
	ok, err := s.repo.Delete(ctx, email, date)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrNotFound
	}
	s.invalidate(email)
	if !day.After(s.today()) {
		return s.triggerRecalc(email), nil
	}
	return false, nil
}

func (s *userLeaveServiceImpl) triggerRecalc(email string) bool {
	if s.onPastChange == nil {
		return false
	}
	go s.onPastChange(email)
	return true
}

func (s *userLeaveServiceImpl) invalidate(email string) {
	s.mu.Lock()
	delete(s.cache, email)
	s.mu.Unlock()
}

// ForEmail = national holidays ∪ the user's leave, cached for cacheTTL. A
// DB error falls back to the national holidays so reminders never stall.
func (s *userLeaveServiceImpl) ForEmail(ctx context.Context, email string) business_hours_dto.HolidayCalendar {
	if email == "" {
		return s.global
	}
	s.mu.Lock()
	if c, ok := s.cache[email]; ok && s.now().Before(c.expires) {
		s.mu.Unlock()
		return c.cal
	}
	s.mu.Unlock()

	dates, err := s.repo.DatesByEmail(ctx, email)
	if err != nil {
		log.Printf("user_leave: leave of %s unavailable, using holidays only: %v", email, err)
		return s.global
	}
	cal := s.global
	if len(dates) > 0 {
		cal = s.global.With(dates)
	}
	s.mu.Lock()
	s.cache[email] = cachedCalendar{cal: cal, expires: s.now().Add(cacheTTL)}
	s.mu.Unlock()
	return cal
}
