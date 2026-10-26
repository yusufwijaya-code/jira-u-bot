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
		ON CONFLICT (email) DO UPDATE SET
			userName = excluded.userName,
			editedBy = excluded.createdBy,
			editedDate = CURRENT_TIMESTAMP
	`
	// createdBy/editedBy are set to the registrant's own email — this is
	// self-registration, there is no separate admin actor (spec Business
	// Logic, POST /register, step 3).
	_, err := r.db.ExecContext(ctx, query, name, email, email)
	return err
}

func (r *registrationRepositoryImpl) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	const query = `
		SELECT userID, userName, email, flagActive, createdBy, createdDate
		FROM ms_user
		WHERE email = ?
	`
	var u registration_dto.User
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&u.UserID, &u.UserName, &u.Email, &u.FlagActive, &u.CreatedBy, &u.CreatedDate,
	)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &u, true, nil
}
