package board_repository

import (
	"context"

	"nudge/modules/board/board_dto"
)

type BoardRepository interface {
	FindByID(ctx context.Context, boardID int64) (board board_dto.Board, found bool, err error)
	Upsert(ctx context.Context, board board_dto.Board) error
}
