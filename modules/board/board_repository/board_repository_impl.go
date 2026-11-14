package board_repository

import (
	"context"
	"database/sql"

	"nudge/modules/board/board_dto"
)

type boardRepositoryImpl struct {
	db *sql.DB
}

func NewBoardRepository(db *sql.DB) BoardRepository {
	return &boardRepositoryImpl{db: db}
}

func (r *boardRepositoryImpl) FindByID(ctx context.Context, boardID int64) (board_dto.Board, bool, error) {
	const query = `SELECT boardID, boardName, projectKey, avatarURL FROM ms_board WHERE boardID = ?`
	var b board_dto.Board
	err := r.db.QueryRowContext(ctx, query, boardID).Scan(&b.BoardID, &b.Name, &b.ProjectKey, &b.AvatarURL)
	if err == sql.ErrNoRows {
		return board_dto.Board{}, false, nil
	}
	if err != nil {
		return board_dto.Board{}, false, err
	}
	return b, true, nil
}

func (r *boardRepositoryImpl) Upsert(ctx context.Context, b board_dto.Board) error {
	const query = `
		INSERT INTO ms_board (boardID, boardName, projectKey, avatarURL)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			boardName = VALUES(boardName),
			projectKey = VALUES(projectKey),
			avatarURL = VALUES(avatarURL)
	`
	_, err := r.db.ExecContext(ctx, query, b.BoardID, b.Name, b.ProjectKey, b.AvatarURL)
	return err
}
