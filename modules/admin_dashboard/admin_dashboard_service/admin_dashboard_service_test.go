package admin_dashboard_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"nudge/modules/google_chat_notifier/google_chat_notifier_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/registration/registration_dto"
)

type fakeRegistrationRepoForAdmin struct {
	users            []registration_dto.User
	toggledUserID    int64
	toggledActive    bool
	updatedAccountID string
	deletedUserID    int64
}

func (f *fakeRegistrationRepoForAdmin) UpsertUser(ctx context.Context, name, email, jiraAccountID string) error {
	return nil
}
func (f *fakeRegistrationRepoForAdmin) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	return nil, false, nil
}
func (f *fakeRegistrationRepoForAdmin) FindUserByJiraAccountID(ctx context.Context, jiraAccountID string) (*registration_dto.User, bool, error) {
	return nil, false, nil
}
func (f *fakeRegistrationRepoForAdmin) UpdateGoogleChatDMSpaceID(ctx context.Context, email, spaceID string) error {
	return nil
}
func (f *fakeRegistrationRepoForAdmin) UpdateReminderTierPreference(ctx context.Context, email, csv string) error {
	return nil
}
func (f *fakeRegistrationRepoForAdmin) FindAll(ctx context.Context) ([]registration_dto.User, error) {
	return f.users, nil
}
func (f *fakeRegistrationRepoForAdmin) UpdateFlagActive(ctx context.Context, userID int64, active bool) error {
	f.toggledUserID = userID
	f.toggledActive = active
	return nil
}
func (f *fakeRegistrationRepoForAdmin) UpdateJiraAccountID(ctx context.Context, userID int64, jiraAccountID string) error {
	f.updatedAccountID = jiraAccountID
	return nil
}
func (f *fakeRegistrationRepoForAdmin) DeleteByID(ctx context.Context, userID int64) error {
	f.deletedUserID = userID
	return nil
}
func (f *fakeRegistrationRepoForAdmin) FindAllWithGoogleChatDMSpaceID(ctx context.Context) ([]registration_dto.User, error) {
	return nil, nil
}

type fakeJiraCredentialForAdmin struct {
	previewByUserID map[int64]string
	deletedUserID   int64
}

func (f *fakeJiraCredentialForAdmin) ResolveCredential(ctx context.Context, email string) (*jira_client_dto.Credential, error) {
	return nil, nil
}
func (f *fakeJiraCredentialForAdmin) UpsertCredential(ctx context.Context, userID int64, jiraEmail, apiToken string) error {
	return nil
}
func (f *fakeJiraCredentialForAdmin) DeleteCredential(ctx context.Context, userID int64) error {
	f.deletedUserID = userID
	return nil
}
func (f *fakeJiraCredentialForAdmin) MaskedPreview(ctx context.Context, userID int64) (string, bool, error) {
	preview, ok := f.previewByUserID[userID]
	return preview, ok, nil
}

type fakeChatDMNotifierForAdmin struct {
	sentTo     []string
	sentText   map[string]string
	failEmails map[string]error
}

func (f *fakeChatDMNotifierForAdmin) SendReminder(ctx context.Context, msg google_chat_notifier_dto.ReminderMessage) error {
	return nil
}
func (f *fakeChatDMNotifierForAdmin) SendPlainTextDM(ctx context.Context, email, text string) error {
	if err, ok := f.failEmails[email]; ok {
		return err
	}
	f.sentTo = append(f.sentTo, email)
	if f.sentText == nil {
		f.sentText = map[string]string{}
	}
	f.sentText[email] = text
	return nil
}

func TestListUsers_IncludesCredentialStatus(t *testing.T) {
	regRepo := &fakeRegistrationRepoForAdmin{users: []registration_dto.User{
		{UserID: 1, Email: "a@esb.co.id", FlagActive: true, CreatedDate: time.Now()},
		{UserID: 2, Email: "b@esb.co.id", FlagActive: true, CreatedDate: time.Now()},
	}}
	credSvc := &fakeJiraCredentialForAdmin{previewByUserID: map[int64]string{1: "••••••••ab3f"}}
	svc := NewAdminDashboardService(regRepo, credSvc, nil, "")

	rows, err := svc.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if !rows[0].HasCredential || rows[0].CredentialMasked != "••••••••ab3f" {
		t.Errorf("rows[0] = %+v, want HasCredential=true masked=••••••••ab3f", rows[0])
	}
	if rows[1].HasCredential {
		t.Error("rows[1].HasCredential = true, want false (no credential set)")
	}
}

func TestSetActive_RejectsSelfModification(t *testing.T) {
	svc := NewAdminDashboardService(&fakeRegistrationRepoForAdmin{}, &fakeJiraCredentialForAdmin{}, nil, "")

	err := svc.SetActive(context.Background(), 1, 1, false)
	if !errors.Is(err, ErrCannotModifySelf) {
		t.Errorf("err = %v, want ErrCannotModifySelf", err)
	}
}

func TestSetActive_AllowsModifyingOtherUser(t *testing.T) {
	regRepo := &fakeRegistrationRepoForAdmin{}
	svc := NewAdminDashboardService(regRepo, &fakeJiraCredentialForAdmin{}, nil, "")

	if err := svc.SetActive(context.Background(), 1, 2, false); err != nil {
		t.Fatalf("SetActive returned error: %v", err)
	}
	if regRepo.toggledUserID != 2 || regRepo.toggledActive != false {
		t.Errorf("toggledUserID=%d toggledActive=%v, want 2, false", regRepo.toggledUserID, regRepo.toggledActive)
	}
}

func TestDeleteUser_RejectsSelfDeletion(t *testing.T) {
	svc := NewAdminDashboardService(&fakeRegistrationRepoForAdmin{}, &fakeJiraCredentialForAdmin{}, nil, "")

	err := svc.DeleteUser(context.Background(), 1, 1)
	if !errors.Is(err, ErrCannotModifySelf) {
		t.Errorf("err = %v, want ErrCannotModifySelf", err)
	}
}

func TestDeleteUser_AllowsDeletingOtherUser(t *testing.T) {
	regRepo := &fakeRegistrationRepoForAdmin{}
	svc := NewAdminDashboardService(regRepo, &fakeJiraCredentialForAdmin{}, nil, "")

	if err := svc.DeleteUser(context.Background(), 1, 2); err != nil {
		t.Fatalf("DeleteUser returned error: %v", err)
	}
	if regRepo.deletedUserID != 2 {
		t.Errorf("deletedUserID = %d, want 2", regRepo.deletedUserID)
	}
}

func TestUpdateJiraAccountID_UpdatesRepository(t *testing.T) {
	regRepo := &fakeRegistrationRepoForAdmin{}
	svc := NewAdminDashboardService(regRepo, &fakeJiraCredentialForAdmin{}, nil, "")

	if err := svc.UpdateJiraAccountID(context.Background(), 2, "acc-new"); err != nil {
		t.Fatalf("UpdateJiraAccountID returned error: %v", err)
	}
	if regRepo.updatedAccountID != "acc-new" {
		t.Errorf("updatedAccountID = %q, want acc-new", regRepo.updatedAccountID)
	}
}

func TestDeleteUserCredential_DelegatesToJiraCredentialService(t *testing.T) {
	credSvc := &fakeJiraCredentialForAdmin{}
	svc := NewAdminDashboardService(&fakeRegistrationRepoForAdmin{}, credSvc, nil, "")

	if err := svc.DeleteUserCredential(context.Background(), 2); err != nil {
		t.Fatalf("DeleteUserCredential returned error: %v", err)
	}
	if credSvc.deletedUserID != 2 {
		t.Errorf("deletedUserID = %d, want 2", credSvc.deletedUserID)
	}
}

func TestBroadcastMessage_SendsToActiveUsersOnlyAndSkipsSelf(t *testing.T) {
	regRepo := &fakeRegistrationRepoForAdmin{users: []registration_dto.User{
		{UserID: 1, Email: "active@esb.co.id", FlagActive: true},
		{UserID: 2, Email: "inactive@esb.co.id", FlagActive: false},
		{UserID: 3, Email: "yusuf.wijaya@esb.co.id", FlagActive: true}, // the bot operator
	}}
	dmNotifier := &fakeChatDMNotifierForAdmin{}
	svc := NewAdminDashboardService(regRepo, &fakeJiraCredentialForAdmin{}, dmNotifier, "yusuf.wijaya@esb.co.id")

	result, err := svc.BroadcastMessage(context.Background(), "Ada fitur baru!")
	if err != nil {
		t.Fatalf("BroadcastMessage returned error: %v", err)
	}

	if result.SentCount != 1 {
		t.Errorf("SentCount = %d, want 1", result.SentCount)
	}
	if len(dmNotifier.sentTo) != 1 || dmNotifier.sentTo[0] != "active@esb.co.id" {
		t.Errorf("sentTo = %v, want [active@esb.co.id] (inactive user and the bot operator's own account must be skipped)", dmNotifier.sentTo)
	}
	if dmNotifier.sentText["active@esb.co.id"] != "Ada fitur baru!" {
		t.Errorf("sentText = %q, want the broadcast message verbatim", dmNotifier.sentText["active@esb.co.id"])
	}
}

func TestBroadcastMessage_CollectsFailedEmailsWithoutStoppingOthers(t *testing.T) {
	regRepo := &fakeRegistrationRepoForAdmin{users: []registration_dto.User{
		{UserID: 1, Email: "a@esb.co.id", FlagActive: true},
		{UserID: 2, Email: "b@esb.co.id", FlagActive: true},
	}}
	dmNotifier := &fakeChatDMNotifierForAdmin{failEmails: map[string]error{"a@esb.co.id": errors.New("chat api down")}}
	svc := NewAdminDashboardService(regRepo, &fakeJiraCredentialForAdmin{}, dmNotifier, "")

	result, err := svc.BroadcastMessage(context.Background(), "Ada fitur baru!")
	if err != nil {
		t.Fatalf("BroadcastMessage returned error: %v", err)
	}

	if result.SentCount != 1 {
		t.Errorf("SentCount = %d, want 1 (b@esb.co.id still succeeds despite a@esb.co.id failing)", result.SentCount)
	}
	if len(result.FailedEmails) != 1 || result.FailedEmails[0] != "a@esb.co.id" {
		t.Errorf("FailedEmails = %v, want [a@esb.co.id]", result.FailedEmails)
	}
}

func TestBroadcastMessage_ReturnsErrDMNotConfiguredWhenNotifierNil(t *testing.T) {
	svc := NewAdminDashboardService(&fakeRegistrationRepoForAdmin{}, &fakeJiraCredentialForAdmin{}, nil, "")

	_, err := svc.BroadcastMessage(context.Background(), "Ada fitur baru!")
	if !errors.Is(err, ErrDMNotConfigured) {
		t.Errorf("err = %v, want ErrDMNotConfigured", err)
	}
}
