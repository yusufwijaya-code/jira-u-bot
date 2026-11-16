package chat_app_repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"nudge/modules/chat_app/chat_app_dto"
)

type repositoryImpl struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) Repository {
	return &repositoryImpl{db: db}
}

const stateColumns = `email, chatAppSpaceID, chatAppUserID, chatAppActivatedDate, chatSender, chatGroupAddedDate, chatGroupError`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanState(row rowScanner) (chat_app_dto.State, error) {
	var s chat_app_dto.State
	var space, user, sender, groupErr sql.NullString
	var activated, groupAdded sql.NullTime
	if err := row.Scan(&s.Email, &space, &user, &activated, &sender, &groupAdded, &groupErr); err != nil {
		return chat_app_dto.State{}, err
	}
	s.SpaceID = space.String
	s.ChatUserID = user.String
	if activated.Valid {
		t := activated.Time
		s.ActivatedDate = &t
	}
	if sender.Valid {
		v := sender.String
		s.Sender = &v
	}
	if groupAdded.Valid {
		t := groupAdded.Time
		s.GroupAddedDate = &t
	}
	s.GroupError = groupErr.String
	return s, nil
}

func (r *repositoryImpl) FindByEmail(ctx context.Context, email string) (chat_app_dto.State, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+stateColumns+` FROM ms_user WHERE email = ?`, email)
	s, err := scanState(row)
	if errors.Is(err, sql.ErrNoRows) {
		return chat_app_dto.State{Email: email}, false, nil
	}
	if err != nil {
		return chat_app_dto.State{}, false, err
	}
	return s, true, nil
}

func (r *repositoryImpl) ListAll(ctx context.Context) (map[string]chat_app_dto.State, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+stateColumns+` FROM ms_user`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]chat_app_dto.State{}
	for rows.Next() {
		s, err := scanState(rows)
		if err != nil {
			return nil, err
		}
		out[strings.ToLower(s.Email)] = s
	}
	return out, rows.Err()
}

func (r *repositoryImpl) SetSpace(ctx context.Context, email, spaceID, chatUserID string) error {
	// MySQL evaluates single-table SET assignments left to right, so the
	// first one still sees the OLD chatAppSpaceID.
	const query = `
		UPDATE ms_user
		SET chatAppActivatedDate = IF(chatAppSpaceID <=> ? AND chatAppActivatedDate IS NOT NULL, chatAppActivatedDate, CURRENT_TIMESTAMP),
		    chatAppSpaceID = ?,
		    chatAppUserID = COALESCE(NULLIF(?, ''), chatAppUserID),
		    editedBy = 'chat_app',
		    editedDate = CURRENT_TIMESTAMP
		WHERE email = ?
	`
	_, err := r.db.ExecContext(ctx, query, spaceID, spaceID, chatUserID, email)
	return err
}

const clearSet = `
	SET chatAppSpaceID = NULL, chatAppUserID = NULL, chatAppActivatedDate = NULL, chatSender = NULL,
	    editedBy = 'chat_app', editedDate = CURRENT_TIMESTAMP
`

func (r *repositoryImpl) ClearBySpace(ctx context.Context, spaceID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ms_user`+clearSet+`WHERE chatAppSpaceID = ?`, spaceID)
	return err
}

func (r *repositoryImpl) ClearByEmail(ctx context.Context, email string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ms_user`+clearSet+`WHERE email = ?`, email)
	return err
}

func (r *repositoryImpl) SetSender(ctx context.Context, email string, sender *string) error {
	var v sql.NullString
	if sender != nil {
		v = sql.NullString{String: *sender, Valid: true}
	}
	const query = `UPDATE ms_user SET chatSender = ?, editedBy = 'chat_app', editedDate = CURRENT_TIMESTAMP WHERE email = ?`
	_, err := r.db.ExecContext(ctx, query, v, email)
	return err
}
