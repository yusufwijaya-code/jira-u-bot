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

func (r *userSettingRepositoryImpl) FindByUserID(ctx context.Context, userID int64) (user_setting_dto.UserSetting, bool, error) {
	const query = `
		SELECT reminderTiers, overageTiers, overageRepeatMinutes, notifyEarlyFinish,
		       notifyReturnedToTodo, mutedUntil, quietHoursStart, quietHoursEnd
		FROM ms_user_setting
		WHERE userID = ?
	`
	var (
		reminderTiers, overageTiers string
		repeat                      sql.NullInt64
		earlyFinish, returnedToTodo bool
		mutedUntil                  sql.NullTime
		quietStart, quietEnd        sql.NullString
	)
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&reminderTiers, &overageTiers, &repeat, &earlyFinish,
		&returnedToTodo, &mutedUntil, &quietStart, &quietEnd,
	)
	if err == sql.ErrNoRows {
		return user_setting_dto.UserSetting{}, false, nil
	}
	if err != nil {
		return user_setting_dto.UserSetting{}, false, err
	}

	setting := user_setting_dto.UserSetting{
		UserID:               userID,
		ReminderTiers:        user_setting_dto.SplitTierCSV(reminderTiers),
		OverageTiers:         user_setting_dto.SplitTierCSV(overageTiers),
		NotifyEarlyFinish:    earlyFinish,
		NotifyReturnedToTodo: returnedToTodo,
	}
	if repeat.Valid {
		minutes := int(repeat.Int64)
		setting.OverageRepeatMinutes = &minutes
	}
	if mutedUntil.Valid {
		until := mutedUntil.Time
		setting.MutedUntil = &until
	}
	if quietStart.Valid && quietEnd.Valid {
		start := user_setting_dto.ClockFromDB(quietStart.String)
		end := user_setting_dto.ClockFromDB(quietEnd.String)
		if start != "" && end != "" {
			setting.QuietHours = &user_setting_dto.QuietHours{Start: start, End: end}
		}
	}
	return setting, true, nil
}

func (r *userSettingRepositoryImpl) UpsertNotification(ctx context.Context, s user_setting_dto.UserSetting) error {
	const query = `
		INSERT INTO ms_user_setting (
			userID, reminderTiers, overageTiers, overageRepeatMinutes,
			notifyEarlyFinish, notifyReturnedToTodo, quietHoursStart, quietHoursEnd
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
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

func (r *userSettingRepositoryImpl) SetMutedUntil(ctx context.Context, userID int64, until *time.Time) error {
	const query = `
		INSERT INTO ms_user_setting (userID, mutedUntil)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE mutedUntil = VALUES(mutedUntil)
	`
	var value sql.NullTime
	if until != nil {
		value = sql.NullTime{Time: *until, Valid: true}
	}
	_, err := r.db.ExecContext(ctx, query, userID, value)
	return err
}
