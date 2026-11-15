package user_setting_service

import (
	"context"
	"log"
	"time"

	"nudge/modules/registration/registration_dto"
	"nudge/modules/user_setting/user_setting_dto"
	"nudge/modules/user_setting/user_setting_repository"
)

// userLookup is the one registration capability this service needs —
// resolving an assignee email to a userID.
type userLookup interface {
	FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error)
}

type userSettingServiceImpl struct {
	repo  user_setting_repository.UserSettingRepository
	users userLookup
}

func NewUserSettingService(repo user_setting_repository.UserSettingRepository, users userLookup) UserSettingService {
	return &userSettingServiceImpl{repo: repo, users: users}
}

func defaultsFor(userID, boardID int64) user_setting_dto.UserSetting {
	s := user_setting_dto.Default()
	s.UserID, s.BoardID = userID, boardID
	s.UseDefault = boardID != user_setting_dto.DefaultBoardID
	return s
}

func (s *userSettingServiceImpl) GetEffectiveForEmail(ctx context.Context, email string, boardID int64) user_setting_dto.UserSetting {
	if email == "" {
		return defaultsFor(0, boardID)
	}
	user, found, err := s.users.FindUserByEmail(ctx, email)
	if err != nil {
		log.Printf("user_setting: lookup %s failed, using defaults: %v", email, err)
		return defaultsFor(0, boardID)
	}
	if !found || user == nil {
		return defaultsFor(0, boardID)
	}
	setting, err := s.GetEffective(ctx, user.UserID, boardID)
	if err != nil {
		log.Printf("user_setting: load settings for %s (board %d) failed, using defaults: %v", email, boardID, err)
		setting = defaultsFor(user.UserID, boardID)
	}
	// The master switch lives on ms_user, already loaded above.
	setting.NotificationsOff = !user.NotificationsEnabled
	return setting
}

func (s *userSettingServiceImpl) NotificationSwitch(ctx context.Context, userID int64) (user_setting_dto.NotificationSwitch, error) {
	sw, found, err := s.repo.FindNotificationSwitch(ctx, userID)
	if err != nil {
		return user_setting_dto.NotificationSwitch{}, err
	}
	if !found {
		return user_setting_dto.NotificationSwitch{}, ErrUserNotFound
	}
	return sw, nil
}

func (s *userSettingServiceImpl) SetNotificationsEnabled(ctx context.Context, userID int64, enabled bool) (user_setting_dto.NotificationSwitch, error) {
	if err := s.repo.SaveNotificationSwitch(ctx, userID, enabled); err != nil {
		return user_setting_dto.NotificationSwitch{}, err
	}
	return s.NotificationSwitch(ctx, userID)
}

func (s *userSettingServiceImpl) GetEffective(ctx context.Context, userID, boardID int64) (user_setting_dto.UserSetting, error) {
	def, found, err := s.repo.FindRow(ctx, userID, user_setting_dto.DefaultBoardID)
	if err != nil {
		return user_setting_dto.UserSetting{}, err
	}
	if !found {
		def = defaultsFor(userID, user_setting_dto.DefaultBoardID)
	}
	def.UseDefault = false
	if boardID == user_setting_dto.DefaultBoardID {
		return def, nil
	}

	board, found, err := s.repo.FindRow(ctx, userID, boardID)
	if err != nil {
		return user_setting_dto.UserSetting{}, err
	}

	eff := def // notification fields + global mute from the default row
	eff.BoardID = boardID
	eff.UseDefault = true
	eff.BoardMutedUntil = nil
	if found {
		eff.BoardMutedUntil = board.MutedUntil
		if !board.UseDefault {
			eff.UseDefault = false
			eff.ReminderTiers = board.ReminderTiers
			eff.OverageTiers = board.OverageTiers
			eff.OverageRepeatMinutes = board.OverageRepeatMinutes
			eff.NotifyStatusChange = board.NotifyStatusChange
			eff.StatusChangeStatuses = board.StatusChangeStatuses
			eff.QuietHours = board.QuietHours
		}
	}
	return eff, nil
}

func (s *userSettingServiceImpl) SaveNotification(ctx context.Context, userID, boardID int64, in user_setting_dto.NotificationInput) (user_setting_dto.UserSetting, error) {
	current, err := s.GetEffective(ctx, userID, boardID)
	if err != nil {
		return user_setting_dto.UserSetting{}, err
	}
	updated, err := in.Apply(current)
	if err != nil {
		return user_setting_dto.UserSetting{}, err
	}
	updated.UserID, updated.BoardID = userID, boardID
	if err := s.repo.UpsertNotification(ctx, updated); err != nil {
		return user_setting_dto.UserSetting{}, err
	}
	updated.UseDefault = false
	return updated, nil
}

func (s *userSettingServiceImpl) ResetToDefault(ctx context.Context, userID, boardID int64) error {
	if boardID == user_setting_dto.DefaultBoardID {
		return ErrDefaultScope
	}
	return s.repo.SetUseDefault(ctx, userID, boardID)
}

func (s *userSettingServiceImpl) MuteUntil(ctx context.Context, userID, boardID int64, until time.Time) error {
	return s.repo.SetMutedUntil(ctx, userID, boardID, &until)
}

func (s *userSettingServiceImpl) Unmute(ctx context.Context, userID, boardID int64) error {
	return s.repo.SetMutedUntil(ctx, userID, boardID, nil)
}

func (s *userSettingServiceImpl) BoardStates(ctx context.Context, userID int64) (map[int64]BoardState, error) {
	rows, err := s.repo.ListRows(ctx, userID)
	if err != nil {
		return nil, err
	}
	states := map[int64]BoardState{}
	for _, row := range rows {
		if row.BoardID == user_setting_dto.DefaultBoardID {
			continue
		}
		states[row.BoardID] = BoardState{BoardID: row.BoardID, Custom: !row.UseDefault, MutedUntil: row.MutedUntil}
	}
	return states, nil
}
