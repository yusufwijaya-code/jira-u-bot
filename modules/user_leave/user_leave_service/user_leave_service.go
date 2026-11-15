package user_leave_service

import (
	"context"
	"errors"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/user_leave/user_leave_dto"
)

var (
	ErrInvalidRange = errors.New("user_leave: invalid date range")
	ErrNoWorkingDay = errors.New("user_leave: no working day in range")
	ErrNotFound     = errors.New("user_leave: leave not found")
)

// UserLeaveService manages a user's own leave days (Tech Spec "Papan Tiket
// & Cuti") and is the per-user CalendarSource for every time calculation:
// a leave day is a non-working day for that user only.
type UserLeaveService interface {
	business_hours_dto.CalendarSource
	List(ctx context.Context, email string) ([]user_leave_dto.Leave, error)
	// Add stores every working day of from..to (to empty = one day).
	Add(ctx context.Context, email, from, to, note string) (user_leave_dto.AddResult, error)
	// Remove deletes one day; recalculating as in AddResult.
	Remove(ctx context.Context, email, date string) (recalculating bool, err error)
}
