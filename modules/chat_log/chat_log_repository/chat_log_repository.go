// Package chat_log_repository stores the "Riwayat pesan" entries in
// tr_chatlog.
package chat_log_repository

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	"nudge/modules/chat_log/chat_log_dto"
)

const (
	defaultLimit  = 50
	maxLimit      = 200
	recordTimeout = 5 * time.Second
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Record inserts e. It is detached from the caller's cancellation (a reply
// is still recorded after the request ends) and only logs failures.
func (r *Repository) Record(ctx context.Context, e chat_log_dto.Entry) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	if err := r.Insert(ctx, e); err != nil {
		log.Printf("chat_log: recording a %s %s for %s failed: %v", e.Direction, e.Kind, e.Email, err)
	}
}

func (r *Repository) Insert(ctx context.Context, e chat_log_dto.Entry) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO tr_chatlog (email, direction, kind, channel, status, body, errorText, createdDate) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.ToLower(strings.TrimSpace(e.Email)), e.Direction, e.Kind, e.Channel, e.Status,
		chat_log_dto.Truncate(e.Body, chat_log_dto.MaxBodyRunes),
		chat_log_dto.Truncate(e.Error, chat_log_dto.MaxErrorRunes),
		// Explicit, so the column follows the connection's loc (WIB) like
		// the rest of the schema instead of the MySQL server clock.
		time.Now(),
	)
	return err
}

func (r *Repository) List(ctx context.Context, f chat_log_dto.Filter) (chat_log_dto.Page, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	where := []string{"1=1"}
	var args []any
	if f.Email != "" {
		where = append(where, "email = ?")
		args = append(args, strings.ToLower(strings.TrimSpace(f.Email)))
	}
	if !f.From.IsZero() {
		where = append(where, "createdDate >= ?")
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		where = append(where, "createdDate < ?")
		args = append(args, f.To)
	}
	if f.BeforeID > 0 {
		where = append(where, "chatLogID < ?")
		args = append(args, f.BeforeID)
	}
	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx,
		`SELECT chatLogID, email, direction, kind, channel, status, body, errorText, createdDate
		   FROM tr_chatlog WHERE `+strings.Join(where, " AND ")+`
		  ORDER BY chatLogID DESC LIMIT ?`, args...)
	if err != nil {
		return chat_log_dto.Page{}, err
	}
	defer rows.Close()

	page := chat_log_dto.Page{Items: []chat_log_dto.Entry{}}
	for rows.Next() {
		var e chat_log_dto.Entry
		if err := rows.Scan(&e.ID, &e.Email, &e.Direction, &e.Kind, &e.Channel, &e.Status, &e.Body, &e.Error, &e.CreatedDate); err != nil {
			return chat_log_dto.Page{}, err
		}
		page.Items = append(page.Items, e)
	}
	if err := rows.Err(); err != nil {
		return chat_log_dto.Page{}, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextBeforeID = page.Items[limit-1].ID
	}
	return page, nil
}

// PurgeBefore deletes entries older than t and returns how many.
func (r *Repository) PurgeBefore(ctx context.Context, t time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tr_chatlog WHERE createdDate < ?`, t)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
