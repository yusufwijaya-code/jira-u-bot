package admin_dashboard_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"jira-u-bot/modules/jira_client/jira_client_dto"
	"jira-u-bot/modules/registration/registration_dto"
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

func TestListUsers_IncludesCredentialStatus(t *testing.T) {
	regRepo := &fakeRegistrationRepoForAdmin{users: []registration_dto.User{
		{UserID: 1, Email: "a@esb.co.id", FlagActive: true, CreatedDate: time.Now()},
		{UserID: 2, Email: "b@esb.co.id", FlagActive: true, CreatedDate: time.Now()},
	}}
	credSvc := &fakeJiraCredentialForAdmin{previewByUserID: map[int64]string{1: "••••••••ab3f"}}
	svc := NewAdminDashboardService(regRepo, credSvc)

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
	svc := NewAdminDashboardService(&fakeRegistrationRepoForAdmin{}, &fakeJiraCredentialForAdmin{})

	err := svc.SetActive(context.Background(), 1, 1, false)
	if !errors.Is(err, ErrCannotModifySelf) {
		t.Errorf("err = %v, want ErrCannotModifySelf", err)
	}
}

func TestSetActive_AllowsModifyingOtherUser(t *testing.T) {
	regRepo := &fakeRegistrationRepoForAdmin{}
	svc := NewAdminDashboardService(regRepo, &fakeJiraCredentialForAdmin{})

	if err := svc.SetActive(context.Background(), 1, 2, false); err != nil {
		t.Fatalf("SetActive returned error: %v", err)
	}
	if regRepo.toggledUserID != 2 || regRepo.toggledActive != false {
		t.Errorf("toggledUserID=%d toggledActive=%v, want 2, false", regRepo.toggledUserID, regRepo.toggledActive)
	}
}

func TestDeleteUser_RejectsSelfDeletion(t *testing.T) {
	svc := NewAdminDashboardService(&fakeRegistrationRepoForAdmin{}, &fakeJiraCredentialForAdmin{})

	err := svc.DeleteUser(context.Background(), 1, 1)
	if !errors.Is(err, ErrCannotModifySelf) {
		t.Errorf("err = %v, want ErrCannotModifySelf", err)
	}
}

func TestDeleteUser_AllowsDeletingOtherUser(t *testing.T) {
	regRepo := &fakeRegistrationRepoForAdmin{}
	svc := NewAdminDashboardService(regRepo, &fakeJiraCredentialForAdmin{})

	if err := svc.DeleteUser(context.Background(), 1, 2); err != nil {
		t.Fatalf("DeleteUser returned error: %v", err)
	}
	if regRepo.deletedUserID != 2 {
		t.Errorf("deletedUserID = %d, want 2", regRepo.deletedUserID)
	}
}

func TestUpdateJiraAccountID_UpdatesRepository(t *testing.T) {
	regRepo := &fakeRegistrationRepoForAdmin{}
	svc := NewAdminDashboardService(regRepo, &fakeJiraCredentialForAdmin{})

	if err := svc.UpdateJiraAccountID(context.Background(), 2, "acc-new"); err != nil {
		t.Fatalf("UpdateJiraAccountID returned error: %v", err)
	}
	if regRepo.updatedAccountID != "acc-new" {
		t.Errorf("updatedAccountID = %q, want acc-new", regRepo.updatedAccountID)
	}
}

func TestDeleteUserCredential_DelegatesToJiraCredentialService(t *testing.T) {
	credSvc := &fakeJiraCredentialForAdmin{}
	svc := NewAdminDashboardService(&fakeRegistrationRepoForAdmin{}, credSvc)

	if err := svc.DeleteUserCredential(context.Background(), 2); err != nil {
		t.Fatalf("DeleteUserCredential returned error: %v", err)
	}
	if credSvc.deletedUserID != 2 {
		t.Errorf("deletedUserID = %d, want 2", credSvc.deletedUserID)
	}
}
