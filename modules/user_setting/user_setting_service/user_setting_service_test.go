package user_setting_service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"nudge/modules/registration/registration_dto"
	"nudge/modules/user_setting/user_setting_dto"
)

type rowKey struct{ user, board int64 }

type fakeRepo struct {
	rows     map[rowKey]user_setting_dto.UserSetting
	findErr  error
	upserted *user_setting_dto.UserSetting
	reset    *rowKey
	switches map[int64]user_setting_dto.NotificationSwitch
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[rowKey]user_setting_dto.UserSetting{}} }

func (f *fakeRepo) FindRow(ctx context.Context, userID, boardID int64) (user_setting_dto.UserSetting, bool, error) {
	if f.findErr != nil {
		return user_setting_dto.UserSetting{}, false, f.findErr
	}
	s, ok := f.rows[rowKey{userID, boardID}]
	return s, ok, nil
}

func (f *fakeRepo) ListRows(ctx context.Context, userID int64) ([]user_setting_dto.UserSetting, error) {
	var out []user_setting_dto.UserSetting
	for k, v := range f.rows {
		if k.user == userID {
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeRepo) UpsertNotification(ctx context.Context, s user_setting_dto.UserSetting) error {
	f.upserted = &s
	s.UseDefault = false
	if old, ok := f.rows[rowKey{s.UserID, s.BoardID}]; ok {
		s.MutedUntil = old.MutedUntil
	} else {
		s.MutedUntil = nil
	}
	f.rows[rowKey{s.UserID, s.BoardID}] = s
	return nil
}

func (f *fakeRepo) SetUseDefault(ctx context.Context, userID, boardID int64) error {
	k := rowKey{userID, boardID}
	f.reset = &k
	if row, ok := f.rows[k]; ok {
		row.UseDefault = true
		f.rows[k] = row
	}
	return nil
}

func (f *fakeRepo) SetMutedUntil(ctx context.Context, userID, boardID int64, until *time.Time) error {
	k := rowKey{userID, boardID}
	row, ok := f.rows[k]
	if !ok {
		row = user_setting_dto.Default()
		row.UserID, row.BoardID, row.UseDefault = userID, boardID, boardID != 0
	}
	row.MutedUntil = until
	f.rows[k] = row
	return nil
}

func (f *fakeRepo) FindNotificationSwitch(ctx context.Context, userID int64) (user_setting_dto.NotificationSwitch, bool, error) {
	if f.findErr != nil {
		return user_setting_dto.NotificationSwitch{}, false, f.findErr
	}
	sw, ok := f.switches[userID]
	return sw, ok, nil
}

func (f *fakeRepo) SaveNotificationSwitch(ctx context.Context, userID int64, enabled bool) error {
	if f.switches == nil {
		f.switches = map[int64]user_setting_dto.NotificationSwitch{}
	}
	f.switches[userID] = user_setting_dto.NotificationSwitch{Enabled: enabled, Chosen: true}
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

func row(userID, boardID int64, tiers ...string) user_setting_dto.UserSetting {
	s := user_setting_dto.Default()
	s.UserID, s.BoardID, s.ReminderTiers = userID, boardID, tiers
	return s
}

func TestGetEffective_DefaultScopeWithoutRowsIsSystemDefault(t *testing.T) {
	got, err := NewUserSettingService(newFakeRepo(), &fakeUsers{}).GetEffective(context.Background(), 1, 0)
	if err != nil || len(got.ReminderTiers) != 6 || got.UseDefault || got.BoardID != 0 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestGetEffective_BoardWithoutRowFollowsTheUsersDefault(t *testing.T) {
	repo := newFakeRepo()
	repo.rows[rowKey{1, 0}] = row(1, 0, "45m")
	got, err := NewUserSettingService(repo, &fakeUsers{}).GetEffective(context.Background(), 1, 28)
	if err != nil || !got.UseDefault || got.BoardID != 28 || !reflect.DeepEqual(got.ReminderTiers, []string{"45m"}) {
		t.Errorf("got %+v, %v; want board 28 following the user's default (45m)", got, err)
	}
}

func TestGetEffective_CustomBoardUsesItsOwnSettings(t *testing.T) {
	repo := newFakeRepo()
	repo.rows[rowKey{1, 0}] = row(1, 0, "45m")
	custom := row(1, 28, "2h")
	custom.UseDefault = false
	repo.rows[rowKey{1, 28}] = custom

	got, _ := NewUserSettingService(repo, &fakeUsers{}).GetEffective(context.Background(), 1, 28)
	if got.UseDefault || !reflect.DeepEqual(got.ReminderTiers, []string{"2h"}) {
		t.Errorf("got %+v; want board 28's own tiers", got)
	}
}

func TestGetEffective_UseDefaultBoardIgnoresItsStoredFieldsButKeepsItsMute(t *testing.T) {
	repo := newFakeRepo()
	repo.rows[rowKey{1, 0}] = row(1, 0, "45m")
	until := time.Now().Add(time.Hour)
	follow := row(1, 28, "2h")
	follow.UseDefault = true
	follow.MutedUntil = &until
	repo.rows[rowKey{1, 28}] = follow

	got, _ := NewUserSettingService(repo, &fakeUsers{}).GetEffective(context.Background(), 1, 28)
	if !got.UseDefault || !reflect.DeepEqual(got.ReminderTiers, []string{"45m"}) {
		t.Errorf("tiers = %v, want the default's 45m", got.ReminderTiers)
	}
	if got.BoardMutedUntil == nil || got.MutedUntil != nil || !got.IsMuted(time.Now()) {
		t.Errorf("mute global %v board %v; want only the board mute", got.MutedUntil, got.BoardMutedUntil)
	}
}

func TestGetEffective_GlobalMuteSilencesEveryBoard(t *testing.T) {
	repo := newFakeRepo()
	until := time.Now().Add(time.Hour)
	def := row(1, 0)
	def.MutedUntil = &until
	repo.rows[rowKey{1, 0}] = def

	got, _ := NewUserSettingService(repo, &fakeUsers{}).GetEffective(context.Background(), 1, 26)
	if !got.IsSilenced(time.Now()) || got.BoardMutedUntil != nil {
		t.Errorf("board 26 must be silenced by the global mute, got %+v", got)
	}
}

func TestGetEffectiveForEmail_DefaultsWhenUnregisteredOrOnError(t *testing.T) {
	svc := NewUserSettingService(newFakeRepo(), &fakeUsers{})
	if got := svc.GetEffectiveForEmail(context.Background(), "nobody@esb.co.id", 28); len(got.ReminderTiers) != 6 || got.BoardID != 28 {
		t.Errorf("unregistered: got %+v", got)
	}
	svc = NewUserSettingService(newFakeRepo(), &fakeUsers{err: errors.New("db down")})
	if got := svc.GetEffectiveForEmail(context.Background(), "a@esb.co.id", 28); len(got.ReminderTiers) != 6 {
		t.Errorf("lookup error: got %+v", got)
	}
	repo := newFakeRepo()
	repo.findErr = errors.New("db down")
	svc = NewUserSettingService(repo, &fakeUsers{users: map[string]registration_dto.User{"a@esb.co.id": {UserID: 1}}})
	if got := svc.GetEffectiveForEmail(context.Background(), "a@esb.co.id", 28); len(got.ReminderTiers) != 6 {
		t.Errorf("load error: got %+v", got)
	}
}

// --- master switch (Tech Spec "Notification On-Off Consent") ---

func TestGetEffectiveForEmail_MasterSwitchSilencesEverything(t *testing.T) {
	users := &fakeUsers{users: map[string]registration_dto.User{
		"off@esb.co.id": {UserID: 1, NotificationsEnabled: false},
		"on@esb.co.id":  {UserID: 2, NotificationsEnabled: true},
	}}
	repo := newFakeRepo()
	svc := NewUserSettingService(repo, users)
	now := time.Now()

	off := svc.GetEffectiveForEmail(context.Background(), "off@esb.co.id", 28)
	if !off.NotificationsOff || !off.IsSilenced(now) {
		t.Errorf("switched-off user: got %+v, want NotificationsOff + silenced", off)
	}
	on := svc.GetEffectiveForEmail(context.Background(), "on@esb.co.id", 28)
	if on.NotificationsOff || on.IsSilenced(now) {
		t.Errorf("switched-on user: got %+v, want not silenced", on)
	}
	// Unregistered → legacy defaults (not silenced).
	if got := svc.GetEffectiveForEmail(context.Background(), "nobody@esb.co.id", 28); got.NotificationsOff {
		t.Errorf("unregistered: got %+v, want notifications on (legacy)", got)
	}
	// A settings load error still honours the switch.
	repo.findErr = errors.New("db down")
	if got := svc.GetEffectiveForEmail(context.Background(), "off@esb.co.id", 28); !got.NotificationsOff {
		t.Errorf("load error: got %+v, want the switch kept off", got)
	}
}

func TestNotificationSwitch_ReadAndSave(t *testing.T) {
	repo := newFakeRepo()
	repo.switches = map[int64]user_setting_dto.NotificationSwitch{1: {Enabled: true, Chosen: false}}
	svc := NewUserSettingService(repo, &fakeUsers{})

	sw, err := svc.NotificationSwitch(context.Background(), 1)
	if err != nil || !sw.Enabled || sw.Chosen {
		t.Fatalf("NotificationSwitch = %+v, %v; want existing user on, not chosen", sw, err)
	}
	if _, err := svc.NotificationSwitch(context.Background(), 99); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("missing user err = %v, want ErrUserNotFound", err)
	}

	sw, err = svc.SetNotificationsEnabled(context.Background(), 1, false)
	if err != nil || sw.Enabled || !sw.Chosen {
		t.Errorf("SetNotificationsEnabled(false) = %+v, %v; want off + chosen", sw, err)
	}
}

func TestSaveNotification_BoardBecomesCustomAndDefaultIsUntouched(t *testing.T) {
	repo := newFakeRepo()
	repo.rows[rowKey{1, 0}] = row(1, 0, "45m")
	svc := NewUserSettingService(repo, &fakeUsers{})

	got, err := svc.SaveNotification(context.Background(), 1, 28, user_setting_dto.NotificationInput{ReminderTiers: []string{"60m"}})
	if err != nil || got.UseDefault || got.BoardID != 28 || got.ReminderTiers[0] != "1h" {
		t.Fatalf("SaveNotification = %+v, %v", got, err)
	}
	if repo.upserted == nil || repo.upserted.BoardID != 28 {
		t.Fatalf("upserted = %+v, want board 28", repo.upserted)
	}
	if !reflect.DeepEqual(repo.rows[rowKey{1, 0}].ReminderTiers, []string{"45m"}) {
		t.Error("saving a board must not change the default row")
	}
}

func TestSaveNotification_ValidatesBeforeWriting(t *testing.T) {
	repo := newFakeRepo()
	_, err := NewUserSettingService(repo, &fakeUsers{}).SaveNotification(context.Background(), 1, 0, user_setting_dto.NotificationInput{ReminderTiers: []string{"9h"}})
	var verr *user_setting_dto.ValidationError
	if !errors.As(err, &verr) || repo.upserted != nil {
		t.Errorf("err = %v, upserted = %v; want ValidationError and no write", err, repo.upserted)
	}
}

func TestResetToDefault(t *testing.T) {
	repo := newFakeRepo()
	svc := NewUserSettingService(repo, &fakeUsers{})
	if err := svc.ResetToDefault(context.Background(), 1, 0); !errors.Is(err, ErrDefaultScope) {
		t.Errorf("default scope: err = %v, want ErrDefaultScope", err)
	}
	if err := svc.ResetToDefault(context.Background(), 1, 28); err != nil || repo.reset == nil || *repo.reset != (rowKey{1, 28}) {
		t.Errorf("board: err %v, reset %v", err, repo.reset)
	}
}

func TestMuteBoardAndGlobalAndBoardStates(t *testing.T) {
	repo := newFakeRepo()
	svc := NewUserSettingService(repo, &fakeUsers{})
	until := time.Now().Add(time.Hour)

	if err := svc.MuteUntil(context.Background(), 1, 28, until); err != nil {
		t.Fatal(err)
	}
	custom := row(1, 26, "2h")
	custom.UseDefault = false
	repo.rows[rowKey{1, 26}] = custom

	states, err := svc.BoardStates(context.Background(), 1)
	if err != nil || len(states) != 2 {
		t.Fatalf("states = %+v, %v", states, err)
	}
	if states[28].Custom || states[28].MutedUntil == nil || !states[26].Custom || states[26].MutedUntil != nil {
		t.Errorf("states = %+v", states)
	}

	if err := svc.Unmute(context.Background(), 1, 28); err != nil || repo.rows[rowKey{1, 28}].MutedUntil != nil {
		t.Errorf("Unmute: err %v, mute %v", err, repo.rows[rowKey{1, 28}].MutedUntil)
	}
}
