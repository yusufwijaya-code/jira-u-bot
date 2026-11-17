package tracking_rule

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

const mysqlDuplicateKeyErrNo = 1062

// The last column is a board of the project with a cached avatar (ms_board),
// so the admin page can show the project picture through the board avatar proxy.
const ruleColumns = `projectKey, projectName, trackedStatuses, doneStatus, reviewStatus, storyPointFields,
	storyPointWriteField, storyPointWriteFieldSubtask, createFields, isActive, editedBy, editedDate,
	(SELECT MIN(b.boardID) FROM ms_board b WHERE b.projectKey = ms_tracking_rule.projectKey AND b.avatarURL <> '')`

// Repository reads and writes ms_tracking_rule.
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

type rowScanner interface{ Scan(dest ...any) error }

func scanRule(row rowScanner) (Rule, error) {
	var r Rule
	var tracked, fields, createFields []byte
	var review sql.NullString
	var avatarBoard sql.NullInt64
	if err := row.Scan(&r.ProjectKey, &r.ProjectName, &tracked, &r.DoneStatus, &review, &fields,
		&r.StoryPointWriteField, &r.StoryPointWriteFieldSubtask, &createFields, &r.IsActive, &r.EditedBy, &r.EditedDate, &avatarBoard); err != nil {
		return Rule{}, err
	}
	r.AvatarBoardID = avatarBoard.Int64
	if err := json.Unmarshal(tracked, &r.TrackedStatuses); err != nil {
		return Rule{}, fmt.Errorf("tracking_rule: trackedStatuses of %s: %w", r.ProjectKey, err)
	}
	if err := json.Unmarshal(fields, &r.StoryPointFields); err != nil {
		return Rule{}, fmt.Errorf("tracking_rule: storyPointFields of %s: %w", r.ProjectKey, err)
	}
	if len(createFields) > 0 {
		if err := json.Unmarshal(createFields, &r.CreateFields); err != nil {
			return Rule{}, fmt.Errorf("tracking_rule: createFields of %s: %w", r.ProjectKey, err)
		}
		if len(r.CreateFields) == 0 {
			r.CreateFields = nil
		}
	}
	r.ReviewStatus = review.String
	return r, nil
}

func (r *Repository) List(ctx context.Context) ([]Rule, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+ruleColumns+` FROM ms_tracking_rule ORDER BY projectKey`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *Repository) Find(ctx context.Context, projectKey string) (Rule, bool, error) {
	rule, err := scanRule(r.db.QueryRowContext(ctx, `SELECT `+ruleColumns+` FROM ms_tracking_rule WHERE projectKey = ?`, projectKey))
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, false, nil
	}
	if err != nil {
		return Rule{}, false, err
	}
	return rule, true, nil
}

// jsonArg encodes a list for a JSON column. Sent as a string: MySQL rejects
// a JSON value built from the binary charset a []byte argument gets.
func jsonArg(list []string) (string, error) {
	if list == nil {
		list = []string{}
	}
	b, err := json.Marshal(list)
	return string(b), err
}

// createFieldsArg is the createFields JSON column value (NULL when empty).
func createFieldsArg(m map[string][]string) (any, error) {
	if len(m) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(m)
	return string(b), err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *Repository) Insert(ctx context.Context, rule Rule) error {
	tracked, err := jsonArg(rule.TrackedStatuses)
	if err != nil {
		return err
	}
	fields, err := jsonArg(rule.StoryPointFields)
	if err != nil {
		return err
	}
	createFields, err := createFieldsArg(rule.CreateFields)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO ms_tracking_rule
		(projectKey, projectName, trackedStatuses, doneStatus, reviewStatus, storyPointFields, storyPointWriteField, storyPointWriteFieldSubtask, createFields, isActive, editedBy)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ProjectKey, rule.ProjectName, tracked, rule.DoneStatus, nullIfEmpty(rule.ReviewStatus), fields,
		rule.StoryPointWriteField, rule.StoryPointWriteFieldSubtask, createFields, rule.IsActive, rule.EditedBy)
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlDuplicateKeyErrNo {
		return ErrRuleExists
	}
	return err
}

// Update rewrites a rule's settings (not isActive). The caller checks that
// the row exists: MySQL reports 0 affected rows for an unchanged row.
func (r *Repository) Update(ctx context.Context, rule Rule) error {
	tracked, err := jsonArg(rule.TrackedStatuses)
	if err != nil {
		return err
	}
	fields, err := jsonArg(rule.StoryPointFields)
	if err != nil {
		return err
	}
	createFields, err := createFieldsArg(rule.CreateFields)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE ms_tracking_rule SET projectName = ?, trackedStatuses = ?, doneStatus = ?, reviewStatus = ?,
		storyPointFields = ?, storyPointWriteField = ?, storyPointWriteFieldSubtask = ?, createFields = ?, editedBy = ?, editedDate = CURRENT_TIMESTAMP
		WHERE projectKey = ?`,
		rule.ProjectName, tracked, rule.DoneStatus, nullIfEmpty(rule.ReviewStatus), fields,
		rule.StoryPointWriteField, rule.StoryPointWriteFieldSubtask, createFields, rule.EditedBy, rule.ProjectKey)
	return err
}

// SetActive switches a rule on/off. Switching off the last active rule
// returns ErrLastActiveRule: the active rows are locked (FOR UPDATE) so two
// concurrent deactivations can't both pass the check (AC4.3).
func (r *Repository) SetActive(ctx context.Context, projectKey string, active bool, editedBy string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if !active {
		rows, err := tx.QueryContext(ctx, `SELECT projectKey FROM ms_tracking_rule WHERE isActive = 1 FOR UPDATE`)
		if err != nil {
			return err
		}
		var keys []string
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				rows.Close()
				return err
			}
			keys = append(keys, key)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(keys) == 1 && keys[0] == projectKey {
			return ErrLastActiveRule
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ms_tracking_rule SET isActive = ?, editedBy = ?, editedDate = CURRENT_TIMESTAMP WHERE projectKey = ?`,
		active, editedBy, projectKey); err != nil {
		return err
	}
	return tx.Commit()
}
