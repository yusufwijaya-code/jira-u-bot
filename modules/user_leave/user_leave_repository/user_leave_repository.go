package user_leave_repository

import (
	"context"
	"database/sql"

	"nudge/modules/user_leave/user_leave_dto"
)

// UserLeaveRepository stores a user's leave days (tr_user_leave). Every
// method is scoped to one email.
type UserLeaveRepository interface {
	// ListByEmail returns the user's leave days, newest first.
	ListByEmail(ctx context.Context, email string) ([]user_leave_dto.Leave, error)
	// Insert stores each date once; it returns the dates that were new.
	Insert(ctx context.Context, email string, dates []string, note string) ([]string, error)
	// Delete removes one day; false when the user had no leave that day.
	Delete(ctx context.Context, email, date string) (bool, error)
	// DatesByEmail returns every leave date ("2006-01-02") of the user.
	DatesByEmail(ctx context.Context, email string) ([]string, error)
}

type userLeaveRepositoryImpl struct{ db *sql.DB }

func NewUserLeaveRepository(db *sql.DB) UserLeaveRepository {
	return &userLeaveRepositoryImpl{db: db}
}

func (r *userLeaveRepositoryImpl) ListByEmail(ctx context.Context, email string) ([]user_leave_dto.Leave, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DATE_FORMAT(leaveDate, '%Y-%m-%d'), COALESCE(note, '') FROM tr_user_leave WHERE email = ? ORDER BY leaveDate DESC`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []user_leave_dto.Leave{}
	for rows.Next() {
		var l user_leave_dto.Leave
		if err := rows.Scan(&l.Date, &l.Note); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *userLeaveRepositoryImpl) Insert(ctx context.Context, email string, dates []string, note string) ([]string, error) {
	var noteVal sql.NullString
	if note != "" {
		noteVal = sql.NullString{String: note, Valid: true}
	}
	var added []string
	for _, d := range dates {
		// INSERT IGNORE on uq_user_leave_email_date: an existing day stays as is.
		res, err := r.db.ExecContext(ctx,
			`INSERT IGNORE INTO tr_user_leave (email, leaveDate, note) VALUES (?, ?, ?)`, email, d, noteVal)
		if err != nil {
			return added, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added = append(added, d)
		}
	}
	return added, nil
}

func (r *userLeaveRepositoryImpl) Delete(ctx context.Context, email, date string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tr_user_leave WHERE email = ? AND leaveDate = ?`, email, date)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *userLeaveRepositoryImpl) DatesByEmail(ctx context.Context, email string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DATE_FORMAT(leaveDate, '%Y-%m-%d') FROM tr_user_leave WHERE email = ?`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
