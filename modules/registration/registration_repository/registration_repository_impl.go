package registration_repository

import (
	"context"
	"database/sql"

	"nudge/modules/registration/registration_dto"
)

type registrationRepositoryImpl struct {
	db *sql.DB
}

func NewRegistrationRepository(db *sql.DB) RegistrationRepository {
	return &registrationRepositoryImpl{db: db}
}

func (r *registrationRepositoryImpl) UpsertUser(ctx context.Context, name, email, jiraAccountID string) error {
	const query = `
		INSERT INTO ms_user (userName, email, jiraAccountId, flagActive, notificationsEnabled, createdBy, createdDate)
		VALUES (?, ?, ?, 1, 0, ?, CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE
			userName = VALUES(userName),
			jiraAccountId = VALUES(jiraAccountId),
			editedBy = VALUES(createdBy),
			editedDate = CURRENT_TIMESTAMP
	`
	// createdBy/editedBy are set to the registrant's own email — this is
	// self-registration, there is no separate admin actor.
	_, err := r.db.ExecContext(ctx, query, name, email, jiraAccountID, email)
	return err
}

const userColumns = `userID, userName, email, flagActive, createdBy, createdDate, googleChatDMSpaceID, jiraAccountId, reminderTierPreference, notificationsEnabled, notificationChoiceDate`

func (r *registrationRepositoryImpl) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	query := `SELECT ` + userColumns + ` FROM ms_user WHERE email = ?`
	row := r.db.QueryRowContext(ctx, query, email)
	return scanUser(row)
}

func (r *registrationRepositoryImpl) FindUserByJiraAccountID(ctx context.Context, jiraAccountID string) (*registration_dto.User, bool, error) {
	query := `SELECT ` + userColumns + ` FROM ms_user WHERE jiraAccountId = ?`
	row := r.db.QueryRowContext(ctx, query, jiraAccountID)
	return scanUser(row)
}

func scanUser(row *sql.Row) (*registration_dto.User, bool, error) {
	var u registration_dto.User
	var dmSpaceID, jiraAccountID, reminderTierPreference sql.NullString
	var choiceDate sql.NullTime
	err := row.Scan(
		&u.UserID, &u.UserName, &u.Email, &u.FlagActive, &u.CreatedBy, &u.CreatedDate, &dmSpaceID, &jiraAccountID, &reminderTierPreference,
		&u.NotificationsEnabled, &choiceDate,
	)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	u.GoogleChatDMSpaceID = dmSpaceID.String
	u.JiraAccountID = jiraAccountID.String
	if reminderTierPreference.Valid {
		u.ReminderTierPreference = &reminderTierPreference.String
	}
	if choiceDate.Valid {
		u.NotificationChoiceDate = &choiceDate.Time
	}
	return &u, true, nil
}

func (r *registrationRepositoryImpl) FindAllWithGoogleChatDMSpaceID(ctx context.Context) ([]registration_dto.User, error) {
	query := `SELECT ` + userColumns + ` FROM ms_user WHERE googleChatDMSpaceID IS NOT NULL AND googleChatDMSpaceID <> ''`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []registration_dto.User
	for rows.Next() {
		var u registration_dto.User
		var dmSpaceID, jiraAccountID, reminderTierPreference sql.NullString
		var choiceDate sql.NullTime
		if err := rows.Scan(&u.UserID, &u.UserName, &u.Email, &u.FlagActive, &u.CreatedBy, &u.CreatedDate, &dmSpaceID, &jiraAccountID, &reminderTierPreference, &u.NotificationsEnabled, &choiceDate); err != nil {
			return nil, err
		}
		u.GoogleChatDMSpaceID = dmSpaceID.String
		u.JiraAccountID = jiraAccountID.String
		if reminderTierPreference.Valid {
			u.ReminderTierPreference = &reminderTierPreference.String
		}
		users = append(users, u)
	}
	return users, rows.Err()
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

func (r *registrationRepositoryImpl) UpdateReminderTierPreference(ctx context.Context, email, csv string) error {
	const query = `
		UPDATE ms_user
		SET reminderTierPreference = ?,
		    editedBy = ?,
		    editedDate = CURRENT_TIMESTAMP
		WHERE email = ?
	`
	_, err := r.db.ExecContext(ctx, query, csv, email, email)
	return err
}

func (r *registrationRepositoryImpl) FindAll(ctx context.Context) ([]registration_dto.User, error) {
	query := `SELECT ` + userColumns + ` FROM ms_user ORDER BY createdDate DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []registration_dto.User
	for rows.Next() {
		var u registration_dto.User
		var dmSpaceID, jiraAccountID, reminderTierPreference sql.NullString
		var choiceDate sql.NullTime
		if err := rows.Scan(&u.UserID, &u.UserName, &u.Email, &u.FlagActive, &u.CreatedBy, &u.CreatedDate, &dmSpaceID, &jiraAccountID, &reminderTierPreference, &u.NotificationsEnabled, &choiceDate); err != nil {
			return nil, err
		}
		u.GoogleChatDMSpaceID = dmSpaceID.String
		u.JiraAccountID = jiraAccountID.String
		if reminderTierPreference.Valid {
			u.ReminderTierPreference = &reminderTierPreference.String
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *registrationRepositoryImpl) UpdateFlagActive(ctx context.Context, userID int64, active bool) error {
	const query = `
		UPDATE ms_user
		SET flagActive = ?, editedBy = 'admin', editedDate = CURRENT_TIMESTAMP
		WHERE userID = ?
	`
	_, err := r.db.ExecContext(ctx, query, active, userID)
	return err
}

func (r *registrationRepositoryImpl) UpdateJiraAccountID(ctx context.Context, userID int64, jiraAccountID string) error {
	const query = `
		UPDATE ms_user
		SET jiraAccountId = ?, editedBy = 'admin', editedDate = CURRENT_TIMESTAMP
		WHERE userID = ?
	`
	_, err := r.db.ExecContext(ctx, query, jiraAccountID, userID)
	return err
}

func (r *registrationRepositoryImpl) DeleteByID(ctx context.Context, userID int64) error {
	const query = `DELETE FROM ms_user WHERE userID = ?`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}
