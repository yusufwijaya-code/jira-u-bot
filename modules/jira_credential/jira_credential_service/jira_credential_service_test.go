package jira_credential_service

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"jira-u-bot/config"
	"jira-u-bot/modules/jira_credential/jira_credential_dto"
	"jira-u-bot/modules/registration/registration_dto"
)

const testKeyBase64 = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" // base64("0123456789abcdef0123456789abcdef")

type fakeJiraCredentialRepo struct {
	byUserID  map[int64]jira_credential_dto.Record
	upsertErr error
	deleteErr error
}

func (f *fakeJiraCredentialRepo) Upsert(ctx context.Context, userID int64, jiraEmail string, encrypted []byte) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	if f.byUserID == nil {
		f.byUserID = map[int64]jira_credential_dto.Record{}
	}
	f.byUserID[userID] = jira_credential_dto.Record{UserID: userID, JiraEmail: jiraEmail, JiraAPITokenEncrypted: encrypted, FlagActive: true}
	return nil
}

func (f *fakeJiraCredentialRepo) FindByUserID(ctx context.Context, userID int64) (*jira_credential_dto.Record, bool, error) {
	rec, ok := f.byUserID[userID]
	if !ok {
		return nil, false, nil
	}
	return &rec, true, nil
}

func (f *fakeJiraCredentialRepo) DeleteByUserID(ctx context.Context, userID int64) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.byUserID, userID)
	return nil
}

func (f *fakeJiraCredentialRepo) FindAll(ctx context.Context) ([]jira_credential_dto.Record, error) {
	var out []jira_credential_dto.Record
	for _, rec := range f.byUserID {
		out = append(out, rec)
	}
	return out, nil
}

type fakeRegistrationRepoForCredential struct {
	usersByEmail map[string]registration_dto.User
}

func (f *fakeRegistrationRepoForCredential) UpsertUser(ctx context.Context, name, email, jiraAccountID string) error {
	return nil
}
func (f *fakeRegistrationRepoForCredential) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	u, ok := f.usersByEmail[email]
	if !ok {
		return nil, false, nil
	}
	return &u, true, nil
}
func (f *fakeRegistrationRepoForCredential) FindUserByJiraAccountID(ctx context.Context, jiraAccountID string) (*registration_dto.User, bool, error) {
	return nil, false, nil
}
func (f *fakeRegistrationRepoForCredential) UpdateGoogleChatDMSpaceID(ctx context.Context, email, spaceID string) error {
	return nil
}

func (f *fakeRegistrationRepoForCredential) UpdateReminderTierPreference(ctx context.Context, email, csv string) error {
	return nil
}
func (f *fakeRegistrationRepoForCredential) FindAllWithGoogleChatDMSpaceID(ctx context.Context) ([]registration_dto.User, error) {
	return nil, nil
}

func newTestConfig() *config.AppConfig {
	return &config.AppConfig{CredentialEncryptionKey: testKeyBase64}
}

func TestUpsertAndResolveCredential_RoundTrips(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	regRepo := &fakeRegistrationRepoForCredential{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 7, Email: "dev@esb.co.id"},
	}}
	svc := NewJiraCredentialService(newTestConfig(), credRepo, regRepo)

	if err := svc.UpsertCredential(context.Background(), 7, "dev@esb.co.id", "my-api-token"); err != nil {
		t.Fatalf("UpsertCredential returned error: %v", err)
	}

	cred, err := svc.ResolveCredential(context.Background(), "dev@esb.co.id")
	if err != nil {
		t.Fatalf("ResolveCredential returned error: %v", err)
	}
	if cred == nil {
		t.Fatal("ResolveCredential returned nil, want the credential just set")
	}
	if cred.APIToken != "my-api-token" || cred.Email != "dev@esb.co.id" {
		t.Errorf("cred = %+v, want {dev@esb.co.id my-api-token}", cred)
	}
}

func TestResolveCredential_ReturnsNilWhenNeverSetUp(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	regRepo := &fakeRegistrationRepoForCredential{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 7, Email: "dev@esb.co.id"},
	}}
	svc := NewJiraCredentialService(newTestConfig(), credRepo, regRepo)

	cred, err := svc.ResolveCredential(context.Background(), "dev@esb.co.id")
	if err != nil {
		t.Fatalf("ResolveCredential returned error: %v, want nil — \"never set up\" is not an error (spec AC5.4)", err)
	}
	if cred != nil {
		t.Errorf("cred = %+v, want nil", cred)
	}
}

func TestResolveCredential_ReturnsNilWhenUnknownEmail(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	regRepo := &fakeRegistrationRepoForCredential{usersByEmail: map[string]registration_dto.User{}}
	svc := NewJiraCredentialService(newTestConfig(), credRepo, regRepo)

	cred, err := svc.ResolveCredential(context.Background(), "ghost@esb.co.id")
	if err != nil || cred != nil {
		t.Errorf("ResolveCredential = %v, %v, want nil, nil", cred, err)
	}
}

func TestDeleteCredential_RemovesIt(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	regRepo := &fakeRegistrationRepoForCredential{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 7, Email: "dev@esb.co.id"},
	}}
	svc := NewJiraCredentialService(newTestConfig(), credRepo, regRepo)

	if err := svc.UpsertCredential(context.Background(), 7, "dev@esb.co.id", "token"); err != nil {
		t.Fatalf("UpsertCredential returned error: %v", err)
	}
	if err := svc.DeleteCredential(context.Background(), 7); err != nil {
		t.Fatalf("DeleteCredential returned error: %v", err)
	}

	cred, err := svc.ResolveCredential(context.Background(), "dev@esb.co.id")
	if err != nil || cred != nil {
		t.Errorf("after delete, ResolveCredential = %v, %v, want nil, nil (spec AC5.5)", cred, err)
	}
}

func TestUpsertCredential_FailsWhenKeyNotConfigured(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	regRepo := &fakeRegistrationRepoForCredential{}
	svc := NewJiraCredentialService(&config.AppConfig{}, credRepo, regRepo) // no CredentialEncryptionKey

	err := svc.UpsertCredential(context.Background(), 7, "dev@esb.co.id", "token")
	if !errors.Is(err, ErrEncryptionKeyNotConfigured) {
		t.Errorf("err = %v, want ErrEncryptionKeyNotConfigured", err)
	}
}

func TestMaskedPreview_ShowsOnlyLastFourCharacters(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	regRepo := &fakeRegistrationRepoForCredential{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 7, Email: "dev@esb.co.id"},
	}}
	svc := NewJiraCredentialService(newTestConfig(), credRepo, regRepo)

	if err := svc.UpsertCredential(context.Background(), 7, "dev@esb.co.id", "ATATT3xFfGF0superSecretTokenAb3f"); err != nil {
		t.Fatalf("UpsertCredential returned error: %v", err)
	}

	preview, found, err := svc.MaskedPreview(context.Background(), 7)
	if err != nil || !found {
		t.Fatalf("MaskedPreview = %q, %v, %v", preview, found, err)
	}
	if preview != "••••••••Ab3f" {
		t.Errorf("preview = %q, want %q (spec AC6.4 — never the full token)", preview, "••••••••Ab3f")
	}
}

func TestMaskedPreview_NotFoundWhenNeverSetUp(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	svc := NewJiraCredentialService(newTestConfig(), credRepo, &fakeRegistrationRepoForCredential{})

	_, found, err := svc.MaskedPreview(context.Background(), 99)
	if err != nil || found {
		t.Errorf("MaskedPreview = found=%v err=%v, want found=false err=nil", found, err)
	}
}

func TestParseKey_RejectsInvalidBase64(t *testing.T) {
	if _, ok := parseKey("not-valid-base64!!!"); ok {
		t.Error("parseKey accepted invalid base64, want ok=false")
	}
}

func TestParseKey_RejectsWrongLength(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString([]byte("too-short"))
	if _, ok := parseKey(shortKey); ok {
		t.Error("parseKey accepted a non-32-byte key, want ok=false")
	}
}
