package user_setting_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"nudge/modules/registration/registration_dto"
	"nudge/modules/user_setting/user_setting_dto"
)

type fakeRepo struct {
	stored    map[int64]user_setting_dto.UserSetting
	findErr   error
	upserted  *user_setting_dto.UserSetting
	mutedID   int64
	mutedTo   *time.Time
	muteCalls int
}

func (f *fakeRepo) FindByUserID(ctx context.Context, userID int64) (user_setting_dto.UserSetting, bool, error) {
	if f.findErr != nil {
		return user_setting_dto.UserSetting{}, false, f.findErr
	}
	s, ok := f.stored[userID]
	return s, ok, nil
}

func (f *fakeRepo) UpsertNotification(ctx context.Context, s user_setting_dto.UserSetting) error {
	f.upserted = &s
	return nil
}

func (f *fakeRepo) SetMutedUntil(ctx context.Context, userID int64, until *time.Time) error {
	f.mutedID, f.mutedTo = userID, until
	f.muteCalls++
	return nil
}

type fakeUsers struct {
	users map[string]registration_dto.User
	err   error
}

func (f *fakeUsers) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	u, ok := f.users[email]
	if !ok {
		return nil, false, nil
	}
	return &u, true, nil
}

func TestGetForEmail_DefaultsWhenUnregistered(t *testing.T) {
	svc := NewUserSettingService(&fakeRepo{}, &fakeUsers{})
	got := svc.GetForEmail(context.Background(), "nobody@esb.co.id")
	if len(got.ReminderTiers) != 6 || *got.OverageRepeatMinutes != 5 {
		t.Errorf("got %+v, want defaults", got)
	}
}

func TestGetForEmail_DefaultsOnLookupOrLoadError(t *testing.T) {
	users := &fakeUsers{err: errors.New("db down")}
	got := NewUserSettingService(&fakeRepo{}, users).GetForEmail(context.Background(), "a@esb.co.id")
	if len(got.ReminderTiers) != 6 {
		t.Errorf("lookup error: got %+v, want defaults", got)
	}

	users = &fakeUsers{users: map[string]registration_dto.User{"a@esb.co.id": {UserID: 1}}}
	got = NewUserSettingService(&fakeRepo{findErr: errors.New("db down")}, users).GetForEmail(context.Background(), "a@esb.co.id")
	if len(got.ReminderTiers) != 6 {
		t.Errorf("load error: got %+v, want defaults", got)
	}
}

func TestGetForEmail_ReturnsStoredSettings(t *testing.T) {
	stored := user_setting_dto.Default()
	stored.UserID = 1
	stored.ReminderTiers = []string{"45m"}
	repo := &fakeRepo{stored: map[int64]user_setting_dto.UserSetting{1: stored}}
	users := &fakeUsers{users: map[string]registration_dto.User{"a@esb.co.id": {UserID: 1}}}

	got := NewUserSettingService(repo, users).GetForEmail(context.Background(), "a@esb.co.id")
	if len(got.ReminderTiers) != 1 || got.ReminderTiers[0] != "45m" {
		t.Errorf("got %v, want [45m]", got.ReminderTiers)
	}
}

func TestSaveNotification_ValidatesBeforeWriting(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewUserSettingService(repo, &fakeUsers{})

	_, err := svc.SaveNotification(context.Background(), 1, user_setting_dto.NotificationInput{ReminderTiers: []string{"9h"}})
	var verr *user_setting_dto.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	if repo.upserted != nil {
		t.Error("an invalid input must not be written")
	}
}

func TestSaveNotification_WritesNormalizedSettings(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewUserSettingService(repo, &fakeUsers{})
	repeat := 15

	got, err := svc.SaveNotification(context.Background(), 9, user_setting_dto.NotificationInput{
		ReminderTiers:        []string{"30m", "60m"},
		OverageRepeatMinutes: &repeat,
		NotifyEarlyFinish:    true,
	})
	if err != nil {
		t.Fatalf("SaveNotification: %v", err)
	}
	if repo.upserted == nil || repo.upserted.UserID != 9 {
		t.Fatalf("upserted = %+v, want row for user 9", repo.upserted)
	}
	if got.ReminderTiers[0] != "1h" || got.ReminderTiers[1] != "30m" {
		t.Errorf("tiers = %v, want [1h 30m]", got.ReminderTiers)
	}
}

func TestMuteAndUnmute(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewUserSettingService(repo, &fakeUsers{})
	until := time.Now().Add(time.Hour)

	if err := svc.MuteUntil(context.Background(), 3, until); err != nil || repo.mutedTo == nil || !repo.mutedTo.Equal(until) {
		t.Errorf("MuteUntil: err %v, stored %v", err, repo.mutedTo)
	}
	if err := svc.Unmute(context.Background(), 3); err != nil || repo.mutedTo != nil {
		t.Errorf("Unmute: err %v, stored %v", err, repo.mutedTo)
	}
}
