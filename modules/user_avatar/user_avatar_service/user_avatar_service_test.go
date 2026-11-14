package user_avatar_service

import (
	"context"
	"errors"
	"testing"
)

type fakeRepo struct {
	byUser  map[int64]string
	byEmail map[string]string
	saved   map[int64]string
}

func (f *fakeRepo) Save(ctx context.Context, userID int64, pictureURL string) error {
	if f.saved == nil {
		f.saved = map[int64]string{}
	}
	f.saved[userID] = pictureURL
	return nil
}
func (f *fakeRepo) FindByUserID(ctx context.Context, userID int64) (string, error) {
	return f.byUser[userID], nil
}
func (f *fakeRepo) FindByEmail(ctx context.Context, email string) (string, error) {
	return f.byEmail[email], nil
}

type fakeFetcher struct {
	err  error
	urls []string
}

func (f *fakeFetcher) Fetch(ctx context.Context, imageURL string) ([]byte, string, error) {
	f.urls = append(f.urls, imageURL)
	if f.err != nil {
		return nil, "", f.err
	}
	return []byte("JPG"), "image/jpeg", nil
}

const pic = "https://lh3.googleusercontent.com/a/ACg8oc=s96-c"

func TestIsGooglePictureURL(t *testing.T) {
	cases := map[string]bool{
		pic: true,
		"https://lh5.googleusercontent.com/-abc/photo.jpg": true,
		"http://lh3.googleusercontent.com/a/x":             false, // not https
		"https://googleusercontent.com.evil.com/a":         false, // look-alike
		"https://evil.com/.googleusercontent.com/a":        false,
		"https://user:pw@lh3.googleusercontent.com/a":      false, // userinfo
		"https://avatar-management.atl-paas.net/a.png":     false, // Jira's CDN is no longer a source
		"": false,
	}
	for u, want := range cases {
		if got := IsGooglePictureURL(u); got != want {
			t.Errorf("IsGooglePictureURL(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestSavePicture_StoresOnlyGoogleURLs(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewUserAvatarService(repo, &fakeFetcher{})

	svc.SavePicture(context.Background(), 1, pic)
	svc.SavePicture(context.Background(), 2, "https://evil.com/a.png")

	if repo.saved[1] != pic || repo.saved[2] != "" {
		t.Errorf("saved = %v, want user 1 = Google URL, user 2 = empty", repo.saved)
	}
}

func TestByEmail_FetchesAndCaches(t *testing.T) {
	fetcher := &fakeFetcher{}
	svc := NewUserAvatarService(&fakeRepo{byEmail: map[string]string{"a@esb.co.id": pic}}, fetcher)

	for i := 0; i < 2; i++ {
		body, ct, err := svc.ByEmail(context.Background(), "a@esb.co.id")
		if err != nil || string(body) != "JPG" || ct != "image/jpeg" {
			t.Fatalf("body=%q ct=%q err=%v", body, ct, err)
		}
	}
	if len(fetcher.urls) != 1 {
		t.Errorf("fetched %d times, want 1 (cached)", len(fetcher.urls))
	}
}

func TestNoStoredOrNonGooglePictureIsNoAvatar(t *testing.T) {
	fetcher := &fakeFetcher{}
	svc := NewUserAvatarService(&fakeRepo{byUser: map[int64]string{2: "https://evil.com/a.png"}}, fetcher)

	if _, _, err := svc.ByUserID(context.Background(), 1); !errors.Is(err, ErrNoAvatar) {
		t.Errorf("no picture: err = %v, want ErrNoAvatar", err)
	}
	if _, _, err := svc.ByUserID(context.Background(), 2); !errors.Is(err, ErrNoAvatar) {
		t.Errorf("non-Google picture: err = %v, want ErrNoAvatar", err)
	}
	if len(fetcher.urls) != 0 {
		t.Errorf("must not fetch, fetched %v", fetcher.urls)
	}
}

func TestTransientErrorIsNotCached(t *testing.T) {
	fetcher := &fakeFetcher{err: errors.New("google down")}
	svc := NewUserAvatarService(&fakeRepo{byUser: map[int64]string{1: pic}}, fetcher)

	if _, _, err := svc.ByUserID(context.Background(), 1); err == nil || errors.Is(err, ErrNoAvatar) {
		t.Errorf("err = %v, want the transient error", err)
	}
	fetcher.err = nil
	if _, _, err := svc.ByUserID(context.Background(), 1); err != nil {
		t.Errorf("retry err = %v, want success", err)
	}
}
