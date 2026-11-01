package auth_service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"jira-u-bot/modules/auth/auth_dto"
	"jira-u-bot/modules/registration/registration_dto"
)

type fakeGoogleVerifier struct {
	exchangeErr error
	rawIDToken  string

	verifyErr error
	claims    *auth_dto.GoogleClaims
}

func (f *fakeGoogleVerifier) Exchange(ctx context.Context, code string) (string, error) {
	if f.exchangeErr != nil {
		return "", f.exchangeErr
	}
	return f.rawIDToken, nil
}

func (f *fakeGoogleVerifier) Verify(ctx context.Context, rawIDToken string) (*auth_dto.GoogleClaims, error) {
	if f.verifyErr != nil {
		return nil, f.verifyErr
	}
	return f.claims, nil
}

func TestVerifyGoogleCallback_RejectsWhenExchangeFails(t *testing.T) {
	svc := &authServiceImpl{verifier: &fakeGoogleVerifier{exchangeErr: errors.New("invalid_grant")}}

	_, err := svc.VerifyGoogleCallback(context.Background(), "bad-code")
	if !errors.Is(err, ErrGoogleVerificationFailed) {
		t.Errorf("err = %v, want ErrGoogleVerificationFailed", err)
	}
}

func TestVerifyGoogleCallback_RejectsWhenIDTokenVerificationFails(t *testing.T) {
	svc := &authServiceImpl{verifier: &fakeGoogleVerifier{
		rawIDToken: "raw-token",
		verifyErr:  errors.New("signature mismatch"),
	}}

	_, err := svc.VerifyGoogleCallback(context.Background(), "code")
	if !errors.Is(err, ErrGoogleVerificationFailed) {
		t.Errorf("err = %v, want ErrGoogleVerificationFailed (spec AC1.2)", err)
	}
}

func TestVerifyGoogleCallback_RejectsWrongDomain(t *testing.T) {
	svc := &authServiceImpl{verifier: &fakeGoogleVerifier{
		rawIDToken: "raw-token",
		claims:     &auth_dto.GoogleClaims{Email: "someone@gmail.com", EmailVerified: true, Name: "Someone"},
	}}

	_, err := svc.VerifyGoogleCallback(context.Background(), "code")
	if !errors.Is(err, ErrDomainNotAllowed) {
		t.Errorf("err = %v, want ErrDomainNotAllowed (spec AC1.3)", err)
	}
}

func TestVerifyGoogleCallback_RejectsUnverifiedEmail(t *testing.T) {
	svc := &authServiceImpl{verifier: &fakeGoogleVerifier{
		rawIDToken: "raw-token",
		claims:     &auth_dto.GoogleClaims{Email: "dev@esb.co.id", EmailVerified: false, Name: "Dev"},
	}}

	_, err := svc.VerifyGoogleCallback(context.Background(), "code")
	if !errors.Is(err, ErrDomainNotAllowed) {
		t.Errorf("err = %v, want ErrDomainNotAllowed — unverified email must be rejected same as wrong domain", err)
	}
}

func TestVerifyGoogleCallback_AcceptsValidEsbEmail(t *testing.T) {
	svc := &authServiceImpl{verifier: &fakeGoogleVerifier{
		rawIDToken: "raw-token",
		claims:     &auth_dto.GoogleClaims{Email: "dev@esb.co.id", EmailVerified: true, Name: "Dev"},
	}}

	claims, err := svc.VerifyGoogleCallback(context.Background(), "code")
	if err != nil {
		t.Fatalf("VerifyGoogleCallback returned error: %v, want nil", err)
	}
	if claims.Email != "dev@esb.co.id" {
		t.Errorf("claims.Email = %q, want %q", claims.Email, "dev@esb.co.id")
	}
}

type fakeRegistrationServiceForAuth struct {
	registerCalled bool
	registerErr    error
}

func (f *fakeRegistrationServiceForAuth) Register(ctx context.Context, name, email string) error {
	f.registerCalled = true
	return f.registerErr
}

type fakeRegistrationRepoForAuth struct {
	usersByEmail map[string]*registration_dto.User
	// afterRegister, if set, is what FindUserByEmail returns the SECOND
	// time it's called for an email not initially found — simulates the
	// row that now exists after Register() ran.
	afterRegister *registration_dto.User
	findCalls     int
}

func (f *fakeRegistrationRepoForAuth) UpsertUser(ctx context.Context, name, email, jiraAccountID string) error {
	return nil
}

func (f *fakeRegistrationRepoForAuth) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	f.findCalls++
	if u, ok := f.usersByEmail[email]; ok {
		return u, true, nil
	}
	if f.findCalls > 1 && f.afterRegister != nil {
		return f.afterRegister, true, nil
	}
	return nil, false, nil
}

func (f *fakeRegistrationRepoForAuth) FindUserByJiraAccountID(ctx context.Context, jiraAccountID string) (*registration_dto.User, bool, error) {
	return nil, false, nil
}

func (f *fakeRegistrationRepoForAuth) UpdateGoogleChatDMSpaceID(ctx context.Context, email, spaceID string) error {
	return nil
}

func (f *fakeRegistrationRepoForAuth) UpdateReminderTierPreference(ctx context.Context, email, csv string) error {
	return nil
}

func (f *fakeRegistrationRepoForAuth) FindAllWithGoogleChatDMSpaceID(ctx context.Context) ([]registration_dto.User, error) {
	return nil, nil
}

func TestGetOrCreateUser_RegistersNewUser(t *testing.T) {
	regSvc := &fakeRegistrationServiceForAuth{}
	regRepo := &fakeRegistrationRepoForAuth{
		usersByEmail:  map[string]*registration_dto.User{},
		afterRegister: &registration_dto.User{UserID: 42, Email: "new@esb.co.id", FlagActive: true},
	}
	svc := &authServiceImpl{registrationSvc: regSvc, registrationRepo: regRepo}

	user, err := svc.GetOrCreateUser(context.Background(), "new@esb.co.id", "New Person")
	if err != nil {
		t.Fatalf("GetOrCreateUser returned error: %v", err)
	}
	if !regSvc.registerCalled {
		t.Error("expected Register to be called for a brand-new email (spec AC1.4)")
	}
	if user.UserID != 42 {
		t.Errorf("UserID = %d, want 42", user.UserID)
	}
}

func TestGetOrCreateUser_SkipsRegisterForExistingUser(t *testing.T) {
	regSvc := &fakeRegistrationServiceForAuth{}
	regRepo := &fakeRegistrationRepoForAuth{usersByEmail: map[string]*registration_dto.User{
		"existing@esb.co.id": {UserID: 7, Email: "existing@esb.co.id", FlagActive: true},
	}}
	svc := &authServiceImpl{registrationSvc: regSvc, registrationRepo: regRepo}

	user, err := svc.GetOrCreateUser(context.Background(), "existing@esb.co.id", "Existing")
	if err != nil {
		t.Fatalf("GetOrCreateUser returned error: %v", err)
	}
	if regSvc.registerCalled {
		t.Error("Register must NOT be called for an existing user — would re-trigger the thank-you DM and a redundant Jira lookup every login")
	}
	if user.UserID != 7 {
		t.Errorf("UserID = %d, want 7", user.UserID)
	}
}

func TestGetOrCreateUser_RejectsDeactivatedUser(t *testing.T) {
	regRepo := &fakeRegistrationRepoForAuth{usersByEmail: map[string]*registration_dto.User{
		"off@esb.co.id": {UserID: 9, Email: "off@esb.co.id", FlagActive: false},
	}}
	svc := &authServiceImpl{registrationSvc: &fakeRegistrationServiceForAuth{}, registrationRepo: regRepo}

	_, err := svc.GetOrCreateUser(context.Background(), "off@esb.co.id", "Off")
	if !errors.Is(err, ErrUserDeactivated) {
		t.Errorf("err = %v, want ErrUserDeactivated (spec AC1.4 flagActive check)", err)
	}
}

func TestGetOrCreateUser_PropagatesRegisterError(t *testing.T) {
	regSvc := &fakeRegistrationServiceForAuth{registerErr: errors.New("jira account not found")}
	regRepo := &fakeRegistrationRepoForAuth{usersByEmail: map[string]*registration_dto.User{}}
	svc := &authServiceImpl{registrationSvc: regSvc, registrationRepo: regRepo}

	_, err := svc.GetOrCreateUser(context.Background(), "nojira@esb.co.id", "No Jira")
	if err == nil {
		t.Fatal("expected an error when Register fails, got nil")
	}
}

type fakeAuthSessionRepo struct {
	created   map[string]int64
	createErr error

	findResult *auth_dto.Session
	findFound  bool
	findErr    error

	deletedIDs []string
}

func (f *fakeAuthSessionRepo) Create(ctx context.Context, sessionID string, userID int64, expiresAt time.Time) error {
	if f.createErr != nil {
		return f.createErr
	}
	if f.created == nil {
		f.created = map[string]int64{}
	}
	f.created[sessionID] = userID
	return nil
}

func (f *fakeAuthSessionRepo) FindByID(ctx context.Context, sessionID string) (*auth_dto.Session, bool, error) {
	return f.findResult, f.findFound, f.findErr
}

func (f *fakeAuthSessionRepo) Delete(ctx context.Context, sessionID string) error {
	f.deletedIDs = append(f.deletedIDs, sessionID)
	return nil
}

func TestIssueSession_CreatesSessionForUser(t *testing.T) {
	repo := &fakeAuthSessionRepo{}
	svc := &authServiceImpl{sessionRepo: repo}

	token, err := svc.IssueSession(context.Background(), 123)
	if err != nil {
		t.Fatalf("IssueSession returned error: %v", err)
	}
	if token == "" {
		t.Fatal("IssueSession returned empty token")
	}
	if repo.created[token] != 123 {
		t.Errorf("session created for userID = %d, want 123", repo.created[token])
	}
}

func TestLogout_DeletesSession(t *testing.T) {
	repo := &fakeAuthSessionRepo{}
	svc := &authServiceImpl{sessionRepo: repo}

	if err := svc.Logout(context.Background(), "token-abc"); err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
	if len(repo.deletedIDs) != 1 || repo.deletedIDs[0] != "token-abc" {
		t.Errorf("deletedIDs = %v, want [token-abc]", repo.deletedIDs)
	}
}

func TestAuthCodeURL_IncludesState(t *testing.T) {
	svc := &authServiceImpl{oauthConfig: &oauth2.Config{
		ClientID:    "test-client-id",
		RedirectURL: "https://labcup.tech/auth/google/callback",
		Endpoint:    oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/auth"},
	}}

	url := svc.AuthCodeURL("csrf-state-123")
	if !strings.Contains(url, "state=csrf-state-123") {
		t.Errorf("AuthCodeURL(%q) = %q, want it to contain the state param", "csrf-state-123", url)
	}
}
