package registration_repository

import (
	"context"
	"database/sql"

	"jira-u-bot/modules/registration/registration_dto"
)

type registrationRepositoryImpl struct {
	db *sql.DB
}

func NewRegistrationRepository(db *sql.DB) RegistrationRepository {
	return &registrationRepositoryImpl{db: db}
}

func (r *registrationRepositoryImpl) UpsertUser(ctx context.Context, name, email, googleChatUserID string) error {
	const query = `
		INSERT INTO ms_user (userName, email, googleChatUserID, flagActive, createdBy, createdDate)
		VALUES (?, ?, ?, 1, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (email) DO UPDATE SET
			userName = excluded.userName,
			googleChatUserID = excluded.googleChatUserID,
			editedBy = excluded.createdBy,
			editedDate = CURRENT_TIMESTAMP
	`
	// createdBy/editedBy are set to the registrant's own email — this is
	// self-registration, there is no separate admin actor (spec Business
	// Logic, POST /register, step 3). An empty googleChatUserID is stored
	// as SQL NULL rather than "" — it's optional and most registrants
	// won't have one yet.
	var chatID any
	if googleChatUserID != "" {
		chatID = googleChatUserID
	}
	_, err := r.db.ExecContext(ctx, query, name, email, chatID, email)
	return err
}

func (r *registrationRepositoryImpl) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	const query = `
		SELECT userID, userName, email, flagActive, createdBy, createdDate, googleChatUserID
		FROM ms_user
		WHERE email = ?
	`
	var u registration_dto.User
	var chatID sql.NullString
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&u.UserID, &u.UserName, &u.Email, &u.FlagActive, &u.CreatedBy, &u.CreatedDate, &chatID,
	)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	u.GoogleChatUserID = chatID.String
	return &u, true, nil
}
