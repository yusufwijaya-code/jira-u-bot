package board_service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"nudge/modules/board/board_dto"
	"nudge/modules/board/board_repository"
	"nudge/modules/jira_client/jira_client_dto"
)

const (
	avatarTTL = 24 * time.Hour
	sprintTTL = 10 * time.Minute
)

// agileClient is the slice of jira_client_service.JiraAgileClient this
// service needs.
type agileClient interface {
	GetBoard(ctx context.Context, boardID int64) (jira_client_dto.BoardInfo, error)
	GetActiveSprints(ctx context.Context, boardID int64) ([]jira_client_dto.SprintRef, error)
	FetchAvatar(ctx context.Context, avatarURL string) ([]byte, string, error)
}

type cachedAvatar struct {
	body        []byte
	contentType string
	expires     time.Time
}

type cachedSprint struct {
	sprint  jira_client_dto.SprintRef
	found   bool
	expires time.Time
}

type boardServiceImpl struct {
	repo  board_repository.BoardRepository
	agile agileClient
	now   func() time.Time

	mu      sync.Mutex
	avatars map[int64]cachedAvatar
	sprints map[int64]cachedSprint
}

func NewBoardService(repo board_repository.BoardRepository, agile agileClient) BoardService {
	return &boardServiceImpl{
		repo:    repo,
		agile:   agile,
		now:     time.Now,
		avatars: map[int64]cachedAvatar{},
		sprints: map[int64]cachedSprint{},
	}
}

func (s *boardServiceImpl) EnsureKnown(ctx context.Context, boardID int64) (board_dto.Board, error) {
	if b, found, err := s.repo.FindByID(ctx, boardID); err != nil || found {
		return b, err
	}
	info, err := s.agile.GetBoard(ctx, boardID)
	if err != nil {
		return board_dto.Board{}, err
	}
	b := board_dto.Board{
		BoardID:    boardID,
		Name:       info.Name,
		ProjectKey: info.Location.ProjectKey,
		AvatarURL:  info.Location.AvatarURI,
	}
	if b.Name == "" {
		b.Name = fmt.Sprintf("Board #%d", boardID)
	}
	return b, s.repo.Upsert(ctx, b)
}

func (s *boardServiceImpl) Get(ctx context.Context, boardID int64) (board_dto.Board, bool, error) {
	return s.repo.FindByID(ctx, boardID)
}

func (s *boardServiceImpl) ActiveSprint(ctx context.Context, boardID int64) (jira_client_dto.SprintRef, bool, error) {
	s.mu.Lock()
	if c, ok := s.sprints[boardID]; ok && s.now().Before(c.expires) {
		s.mu.Unlock()
		return c.sprint, c.found, nil
	}
	s.mu.Unlock()

	sprints, err := s.agile.GetActiveSprints(ctx, boardID)
	if err != nil {
		return jira_client_dto.SprintRef{}, false, err
	}
	entry := cachedSprint{expires: s.now().Add(sprintTTL)}
	if len(sprints) > 0 {
		entry.sprint, entry.found = sprints[0], true
		entry.sprint.BoardID = boardID
	}
	s.mu.Lock()
	s.sprints[boardID] = entry
	s.mu.Unlock()
	return entry.sprint, entry.found, nil
}

func (s *boardServiceImpl) Avatar(ctx context.Context, boardID int64) ([]byte, string, error) {
	s.mu.Lock()
	if c, ok := s.avatars[boardID]; ok && s.now().Before(c.expires) {
		s.mu.Unlock()
		return c.body, c.contentType, nil
	}
	s.mu.Unlock()

	b, found, err := s.repo.FindByID(ctx, boardID)
	if err != nil {
		return nil, "", err
	}
	if !found || b.AvatarURL == "" {
		return nil, "", ErrAvatarNotFound
	}
	body, contentType, err := s.agile.FetchAvatar(ctx, b.AvatarURL)
	if err != nil {
		return nil, "", err
	}
	s.mu.Lock()
	s.avatars[boardID] = cachedAvatar{body: body, contentType: contentType, expires: s.now().Add(avatarTTL)}
	s.mu.Unlock()
	return body, contentType, nil
}
