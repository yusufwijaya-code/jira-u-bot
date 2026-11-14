package board_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"nudge/modules/board/board_dto"
	"nudge/modules/jira_client/jira_client_dto"
)

type fakeRepo struct {
	boards  map[int64]board_dto.Board
	upserts int
}

func (f *fakeRepo) FindByID(ctx context.Context, id int64) (board_dto.Board, bool, error) {
	b, ok := f.boards[id]
	return b, ok, nil
}

func (f *fakeRepo) Upsert(ctx context.Context, b board_dto.Board) error {
	f.upserts++
	f.boards[b.BoardID] = b
	return nil
}

type fakeAgile struct {
	boardCalls, sprintCalls, avatarCalls int
	boardErr                             error
	fetchedURL                           string
}

func (f *fakeAgile) GetBoard(ctx context.Context, id int64) (jira_client_dto.BoardInfo, error) {
	f.boardCalls++
	if f.boardErr != nil {
		return jira_client_dto.BoardInfo{}, f.boardErr
	}
	info := jira_client_dto.BoardInfo{ID: id, Name: "ESB OMS Board"}
	info.Location.ProjectKey = "OMS"
	info.Location.AvatarURI = "https://jira/avatar/10417"
	return info, nil
}

func (f *fakeAgile) GetActiveSprints(ctx context.Context, id int64) ([]jira_client_dto.SprintRef, error) {
	f.sprintCalls++
	return []jira_client_dto.SprintRef{{ID: 8294, Name: "OMS Sprint 23 - 2026", State: "active"}}, nil
}

func (f *fakeAgile) FetchAvatar(ctx context.Context, url string) ([]byte, string, error) {
	f.avatarCalls++
	f.fetchedURL = url
	return []byte("<svg/>"), "image/svg+xml", nil
}

func newService(repo *fakeRepo, agile *fakeAgile) *boardServiceImpl {
	return NewBoardService(repo, agile).(*boardServiceImpl)
}

func TestEnsureKnown_FetchesOnceThenUsesCache(t *testing.T) {
	repo := &fakeRepo{boards: map[int64]board_dto.Board{}}
	agile := &fakeAgile{}
	svc := newService(repo, agile)

	for i := 0; i < 3; i++ {
		b, err := svc.EnsureKnown(context.Background(), 28)
		if err != nil || b.Name != "ESB OMS Board" || b.ProjectKey != "OMS" || b.AvatarURL == "" {
			t.Fatalf("EnsureKnown = %+v, %v", b, err)
		}
	}
	if agile.boardCalls != 1 || repo.upserts != 1 {
		t.Errorf("Jira calls = %d, upserts = %d; want 1 each", agile.boardCalls, repo.upserts)
	}
}

func TestEnsureKnown_JiraErrorIsReturnedAndNothingCached(t *testing.T) {
	repo := &fakeRepo{boards: map[int64]board_dto.Board{}}
	svc := newService(repo, &fakeAgile{boardErr: errors.New("jira down")})

	if _, err := svc.EnsureKnown(context.Background(), 28); err == nil {
		t.Error("want the Jira error")
	}
	if len(repo.boards) != 0 {
		t.Error("nothing may be cached on failure")
	}
}

func TestAvatar_UnknownBoardIsNotFoundAndNeverFetched(t *testing.T) {
	agile := &fakeAgile{}
	svc := newService(&fakeRepo{boards: map[int64]board_dto.Board{}}, agile)

	if _, _, err := svc.Avatar(context.Background(), 999); !errors.Is(err, ErrAvatarNotFound) {
		t.Errorf("err = %v, want ErrAvatarNotFound", err)
	}
	if agile.avatarCalls != 0 {
		t.Error("an unknown board must never trigger a fetch")
	}
}

func TestAvatar_FetchesStoredURLAndCachesFor24h(t *testing.T) {
	repo := &fakeRepo{boards: map[int64]board_dto.Board{28: {BoardID: 28, Name: "B", AvatarURL: "https://jira/avatar/10417"}}}
	agile := &fakeAgile{}
	svc := newService(repo, agile)
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		body, ct, err := svc.Avatar(context.Background(), 28)
		if err != nil || string(body) != "<svg/>" || ct != "image/svg+xml" {
			t.Fatalf("Avatar = %q, %q, %v", body, ct, err)
		}
	}
	if agile.avatarCalls != 1 || agile.fetchedURL != "https://jira/avatar/10417" {
		t.Errorf("fetches = %d (%s); want 1 of the stored URL", agile.avatarCalls, agile.fetchedURL)
	}

	now = now.Add(25 * time.Hour)
	svc.Avatar(context.Background(), 28)
	if agile.avatarCalls != 2 {
		t.Errorf("fetches after 25h = %d, want 2 (cache expired)", agile.avatarCalls)
	}
}

func TestActiveSprint_CachedBriefly(t *testing.T) {
	agile := &fakeAgile{}
	svc := newService(&fakeRepo{boards: map[int64]board_dto.Board{}}, agile)

	for i := 0; i < 3; i++ {
		s, found, err := svc.ActiveSprint(context.Background(), 28)
		if err != nil || !found || s.ID != 8294 || s.BoardID != 28 {
			t.Fatalf("ActiveSprint = %+v, %v, %v", s, found, err)
		}
	}
	if agile.sprintCalls != 1 {
		t.Errorf("Jira calls = %d, want 1", agile.sprintCalls)
	}
}
