package user_setting_repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"nudge/modules/user_setting/user_setting_dto"
)

type userSettingRepositoryImpl struct {
	db *sql.DB
}

func NewUserSettingRepository(db *sql.DB) UserSettingRepository {
	return &userSettingRepositoryImpl{db: db}
}

const rowColumns = `userID, boardID, useDefault, reminderTiers, overageTiers, overageRepeatMinutes,
	notifyEarlyFinish, notifyReturnedToTodo, mutedUntil, quietHoursStart, quietHoursEnd`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRow(scanner rowScanner) (user_setting_dto.UserSetting, error) {
	var (
		s                           user_setting_dto.UserSetting
		reminderTiers, overageTiers string
		repeat                      sql.NullInt64
		mutedUntil                  sql.NullTime
		quietStart, quietEnd        sql.NullString
	)
	err := scanner.Scan(
		&s.UserID, &s.BoardID, &s.UseDefault, &reminderTiers, &overageTiers, &repeat,
		&s.NotifyEarlyFinish, &s.NotifyReturnedToTodo, &mutedUntil, &quietStart, &quietEnd,
	)
	if err != nil {
		return user_setting_dto.UserSetting{}, err
	}
	s.ReminderTiers = user_setting_dto.SplitTierCSV(reminderTiers)
	s.OverageTiers = user_setting_dto.SplitTierCSV(overageTiers)
	if repeat.Valid {
		minutes := int(repeat.Int64)
		s.OverageRepeatMinutes = &minutes
	}
	if mutedUntil.Valid {
		until := mutedUntil.Time
		s.MutedUntil = &until
	}
	if quietStart.Valid && quietEnd.Valid {
		start := user_setting_dto.ClockFromDB(quietStart.String)
		end := user_setting_dto.ClockFromDB(quietEnd.String)
		if start != "" && end != "" {
			s.QuietHours = &user_setting_dto.QuietHours{Start: start, End: end}
		}
	}
	return s, nil
}

func (r *userSettingRepositoryImpl) FindRow(ctx context.Context, userID, boardID int64) (user_setting_dto.UserSetting, bool, error) {
	query := `SELECT ` + rowColumns + ` FROM ms_user_setting WHERE userID = ? AND boardID = ?`
	s, err := scanRow(r.db.QueryRowContext(ctx, query, userID, boardID))
	if err == sql.ErrNoRows {
		return user_setting_dto.UserSetting{}, false, nil
	}
	if err != nil {
		return user_setting_dto.UserSetting{}, false, err
	}
	return s, true, nil
}

func (r *userSettingRepositoryImpl) ListRows(ctx context.Context, userID int64) ([]user_setting_dto.UserSetting, error) {
	query := `SELECT ` + rowColumns + ` FROM ms_user_setting WHERE userID = ? ORDER BY boardID`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []user_setting_dto.UserSetting
	for rows.Next() {
		s, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *userSettingRepositoryImpl) UpsertNotification(ctx context.Context, s user_setting_dto.UserSetting) error {
	const query = `
		INSERT INTO ms_user_setting (
			userID, boardID, useDefault, reminderTiers, overageTiers, overageRepeatMinutes,
			notifyEarlyFinish, notifyReturnedToTodo, quietHoursStart, quietHoursEnd
		) VALUES (?, ?, 0, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			useDefault = 0,
			reminderTiers = VALUES(reminderTiers),
			overageTiers = VALUES(overageTiers),
			overageRepeatMinutes = VALUES(overageRepeatMinutes),
			notifyEarlyFinish = VALUES(notifyEarlyFinish),
			notifyReturnedToTodo = VALUES(notifyReturnedToTodo),
			quietHoursStart = VALUES(quietHoursStart),
			quietHoursEnd = VALUES(quietHoursEnd)
	`
	var repeat sql.NullInt64
	if s.OverageRepeatMinutes != nil {
		repeat = sql.NullInt64{Int64: int64(*s.OverageRepeatMinutes), Valid: true}
	}
	var quietStart, quietEnd sql.NullString
	if s.QuietHours != nil {
		quietStart = sql.NullString{String: s.QuietHours.Start, Valid: true}
		quietEnd = sql.NullString{String: s.QuietHours.End, Valid: true}
	}
	_, err := r.db.ExecContext(ctx, query,
		s.UserID,
		s.BoardID,
		strings.Join(s.ReminderTiers, ","),
		strings.Join(s.OverageTiers, ","),
		repeat,
		s.NotifyEarlyFinish,
		s.NotifyReturnedToTodo,
		quietStart,
		quietEnd,
	)
	return err
}

func (r *userSettingRepositoryImpl) SetUseDefault(ctx context.Context, userID, boardID int64) error {
	const query = `UPDATE ms_user_setting SET useDefault = 1 WHERE userID = ? AND boardID = ?`
	_, err := r.db.ExecContext(ctx, query, userID, boardID)
	return err
}

func (r *userSettingRepositoryImpl) SetMutedUntil(ctx context.Context, userID, boardID int64, until *time.Time) error {
	// A board row created only to hold a mute keeps following the default
	// (useDefault = 1); the default row ignores useDefault.
	const query = `
		INSERT INTO ms_user_setting (userID, boardID, useDefault, mutedUntil)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE mutedUntil = VALUES(mutedUntil)
	`
	var value sql.NullTime
	if until != nil {
		value = sql.NullTime{Time: *until, Valid: true}
	}
	useDefault := boardID != user_setting_dto.DefaultBoardID
	_, err := r.db.ExecContext(ctx, query, userID, boardID, useDefault, value)
	return err
}
