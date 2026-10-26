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

func (r *registrationRepositoryImpl) UpsertUser(ctx context.Context, name, email string) error {
	const query = `
		INSERT INTO ms_user (userName, email, flagActive, createdBy, createdDate)
		VALUES (?, ?, 1, ?, CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE
			userName = VALUES(userName),
			editedBy = VALUES(createdBy),
			editedDate = CURRENT_TIMESTAMP
	`
	// createdBy/editedBy are set to the registrant's own email — this is
	// self-registration, there is no separate admin actor.
	_, err := r.db.ExecContext(ctx, query, name, email, email)
	return err
}

func (r *registrationRepositoryImpl) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	const query = `
		SELECT userID, userName, email, flagActive, createdBy, createdDate, googleChatDMSpaceID
		FROM ms_user
		WHERE email = ?
	`
	var u registration_dto.User
	var dmSpaceID sql.NullString
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&u.UserID, &u.UserName, &u.Email, &u.FlagActive, &u.CreatedBy, &u.CreatedDate, &dmSpaceID,
	)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	u.GoogleChatDMSpaceID = dmSpaceID.String
	return &u, true, nil
}

func (r *registrationRepositoryImpl) UpdateGoogleChatDMSpaceID(ctx context.Context, email, spaceID string) error {
	const query = `
		UPDATE ms_user
		SET googleChatDMSpaceID = ?,
		    editedBy = 'chat_dm_notifier',
		    editedDate = CURRENT_TIMESTAMP
		WHERE email = ?
	`
	_, err := r.db.ExecContext(ctx, query, spaceID, email)
	return err
}
