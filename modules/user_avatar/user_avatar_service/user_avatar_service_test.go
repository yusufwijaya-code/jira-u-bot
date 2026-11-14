package user_avatar_service

import (
	"context"
	"errors"
	"testing"

	"nudge/modules/registration/registration_dto"
	"nudge/modules/registration/registration_repository"
)

type fakeUsers struct {
	registration_repository.RegistrationRepository
	users []registration_dto.User
}

func (f *fakeUsers) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	for i := range f.users {
		if f.users[i].Email == email {
			return &f.users[i], true, nil
		}
	}
	return nil, false, nil
}

func (f *fakeUsers) FindAll(ctx context.Context) ([]registration_dto.User, error) {
	return f.users, nil
}

type fakeJira struct {
	url       string
	urlErr    error
	urlCalls  int
	fetchErr  error
	fetchURLs []string
}

func (f *fakeJira) UserAvatarURL(ctx context.Context, accountID string) (string, error) {
	f.urlCalls++
	return f.url, f.urlErr
}

func (f *fakeJira) FetchUserAvatar(ctx context.Context, avatarURL string) ([]byte, string, error) {
	f.fetchURLs = append(f.fetchURLs, avatarURL)
	if f.fetchErr != nil {
		return nil, "", f.fetchErr
	}
	return []byte("PNG"), "image/png", nil
}

var users = &fakeUsers{users: []registration_dto.User{
	{UserID: 1, Email: "a@esb.co.id", JiraAccountID: "acc-1"},
	{UserID: 2, Email: "b@esb.co.id"},
}}

func TestByEmail_FetchesAndCaches(t *testing.T) {
	jira := &fakeJira{url: "https://x.atl-paas.net/a.png"}
	svc := NewUserAvatarService(users, jira)

	for i := 0; i < 2; i++ {
		body, ct, err := svc.ByEmail(context.Background(), "a@esb.co.id")
		if err != nil || string(body) != "PNG" || ct != "image/png" {
			t.Fatalf("body=%q ct=%q err=%v", body, ct, err)
		}
	}
	if jira.urlCalls != 1 || len(jira.fetchURLs) != 1 {
		t.Errorf("jira calls url=%d fetch=%d, want 1/1 (cached)", jira.urlCalls, len(jira.fetchURLs))
	}
}

func TestByUserID_FindsUser(t *testing.T) {
	svc := NewUserAvatarService(users, &fakeJira{url: "https://x.atl-paas.net/a.png"})
	if _, _, err := svc.ByUserID(context.Background(), 1); err != nil {
		t.Errorf("err = %v", err)
	}
	if _, _, err := svc.ByUserID(context.Background(), 99); !errors.Is(err, ErrNoAvatar) {
		t.Errorf("unknown user err = %v, want ErrNoAvatar", err)
	}
}

func TestNoAccountIDOrNoPicture_IsNoAvatar(t *testing.T) {
	jira := &fakeJira{}
	svc := NewUserAvatarService(users, jira)

	if _, _, err := svc.ByEmail(context.Background(), "b@esb.co.id"); !errors.Is(err, ErrNoAvatar) {
		t.Errorf("no account id: err = %v, want ErrNoAvatar", err)
	}
	if jira.urlCalls != 0 {
		t.Error("must not ask Jira for a user without an account id")
	}
	if _, _, err := svc.ByEmail(context.Background(), "a@esb.co.id"); !errors.Is(err, ErrNoAvatar) {
		t.Errorf("empty avatar url: err = %v, want ErrNoAvatar", err)
	}
	svc.ByEmail(context.Background(), "a@esb.co.id")
	if jira.urlCalls != 1 {
		t.Errorf("missing picture must be cached, url calls = %d", jira.urlCalls)
	}
}

func TestTransientErrorIsNotCached(t *testing.T) {
	jira := &fakeJira{urlErr: errors.New("jira down")}
	svc := NewUserAvatarService(users, jira)

	if _, _, err := svc.ByEmail(context.Background(), "a@esb.co.id"); err == nil || errors.Is(err, ErrNoAvatar) {
		t.Errorf("err = %v, want the transient error", err)
	}
	jira.urlErr, jira.url = nil, "https://x.atl-paas.net/a.png"
	if _, _, err := svc.ByEmail(context.Background(), "a@esb.co.id"); err != nil {
		t.Errorf("retry err = %v, want success", err)
	}
}
