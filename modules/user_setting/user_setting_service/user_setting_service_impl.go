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

func (s *userSettingServiceImpl) GetForEmail(ctx context.Context, email string) user_setting_dto.UserSetting {
	if email == "" {
		return user_setting_dto.Default()
	}
	user, found, err := s.users.FindUserByEmail(ctx, email)
	if err != nil {
		log.Printf("user_setting: lookup %s failed, using defaults: %v", email, err)
		return user_setting_dto.Default()
	}
	if !found || user == nil {
		return user_setting_dto.Default()
	}
	setting, err := s.GetForUser(ctx, user.UserID)
	if err != nil {
		log.Printf("user_setting: load settings for %s failed, using defaults: %v", email, err)
		return user_setting_dto.Default()
	}
	return setting
}

func (s *userSettingServiceImpl) GetForUser(ctx context.Context, userID int64) (user_setting_dto.UserSetting, error) {
	setting, found, err := s.repo.FindByUserID(ctx, userID)
	if err != nil {
		return user_setting_dto.UserSetting{}, err
	}
	if !found {
		setting = user_setting_dto.Default()
		setting.UserID = userID
	}
	return setting, nil
}

func (s *userSettingServiceImpl) SaveNotification(ctx context.Context, userID int64, in user_setting_dto.NotificationInput) (user_setting_dto.UserSetting, error) {
	current, err := s.GetForUser(ctx, userID)
	if err != nil {
		return user_setting_dto.UserSetting{}, err
	}
	updated, err := in.Apply(current)
	if err != nil {
		return user_setting_dto.UserSetting{}, err
	}
	if err := s.repo.UpsertNotification(ctx, updated); err != nil {
		return user_setting_dto.UserSetting{}, err
	}
	return updated, nil
}

func (s *userSettingServiceImpl) MuteUntil(ctx context.Context, userID int64, until time.Time) error {
	return s.repo.SetMutedUntil(ctx, userID, &until)
}

func (s *userSettingServiceImpl) Unmute(ctx context.Context, userID int64) error {
	return s.repo.SetMutedUntil(ctx, userID, nil)
}
