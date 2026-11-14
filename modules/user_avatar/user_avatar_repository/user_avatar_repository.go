// Package user_avatar_repository stores each user's Google profile picture
// URL (ms_user_picture).
package user_avatar_repository

import (
	"context"
	"database/sql"
)

type UserAvatarRepository interface {
	// Save sets userID's picture URL ("" clears it).
	Save(ctx context.Context, userID int64, pictureURL string) error
	// FindByUserID returns the stored URL ("" when none).
	FindByUserID(ctx context.Context, userID int64) (string, error)
	// FindByEmail returns the stored URL of the user with that email.
	FindByEmail(ctx context.Context, email string) (string, error)
}

type userAvatarRepositoryImpl struct {
	db *sql.DB
}

func NewUserAvatarRepository(db *sql.DB) UserAvatarRepository {
	return &userAvatarRepositoryImpl{db: db}
}

func (r *userAvatarRepositoryImpl) Save(ctx context.Context, userID int64, pictureURL string) error {
	const query = `
		INSERT INTO ms_user_picture (userID, pictureURL) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE pictureURL = VALUES(pictureURL)
	`
	_, err := r.db.ExecContext(ctx, query, userID, pictureURL)
	return err
}

func (r *userAvatarRepositoryImpl) FindByUserID(ctx context.Context, userID int64) (string, error) {
	return r.find(ctx, `SELECT pictureURL FROM ms_user_picture WHERE userID = ?`, userID)
}

func (r *userAvatarRepositoryImpl) FindByEmail(ctx context.Context, email string) (string, error) {
	return r.find(ctx, `
		SELECT p.pictureURL FROM ms_user_picture p
		JOIN ms_user u ON u.userID = p.userID
		WHERE u.email = ?
	`, email)
}

func (r *userAvatarRepositoryImpl) find(ctx context.Context, query string, arg any) (string, error) {
	var pictureURL string
	err := r.db.QueryRowContext(ctx, query, arg).Scan(&pictureURL)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return pictureURL, err
}
