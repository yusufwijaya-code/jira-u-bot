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

const ruleColumns = `projectKey, projectName, trackedStatuses, doneStatus, reviewStatus, storyPointFields,
	storyPointWriteField, storyPointWriteFieldSubtask, isActive, editedBy, editedDate`

// Repository reads and writes ms_tracking_rule.
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

type rowScanner interface{ Scan(dest ...any) error }

func scanRule(row rowScanner) (Rule, error) {
	var r Rule
	var tracked, fields []byte
	var review sql.NullString
	if err := row.Scan(&r.ProjectKey, &r.ProjectName, &tracked, &r.DoneStatus, &review, &fields,
		&r.StoryPointWriteField, &r.StoryPointWriteFieldSubtask, &r.IsActive, &r.EditedBy, &r.EditedDate); err != nil {
		return Rule{}, err
	}
	if err := json.Unmarshal(tracked, &r.TrackedStatuses); err != nil {
		return Rule{}, fmt.Errorf("tracking_rule: trackedStatuses of %s: %w", r.ProjectKey, err)
	}
	if err := json.Unmarshal(fields, &r.StoryPointFields); err != nil {
		return Rule{}, fmt.Errorf("tracking_rule: storyPointFields of %s: %w", r.ProjectKey, err)
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
	_, err = r.db.ExecContext(ctx, `INSERT INTO ms_tracking_rule
		(projectKey, projectName, trackedStatuses, doneStatus, reviewStatus, storyPointFields, storyPointWriteField, storyPointWriteFieldSubtask, isActive, editedBy)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ProjectKey, rule.ProjectName, tracked, rule.DoneStatus, nullIfEmpty(rule.ReviewStatus), fields,
		rule.StoryPointWriteField, rule.StoryPointWriteFieldSubtask, rule.IsActive, rule.EditedBy)
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
	_, err = r.db.ExecContext(ctx, `UPDATE ms_tracking_rule SET projectName = ?, trackedStatuses = ?, doneStatus = ?, reviewStatus = ?,
		storyPointFields = ?, storyPointWriteField = ?, storyPointWriteFieldSubtask = ?, editedBy = ?, editedDate = CURRENT_TIMESTAMP
		WHERE projectKey = ?`,
		rule.ProjectName, tracked, rule.DoneStatus, nullIfEmpty(rule.ReviewStatus), fields,
		rule.StoryPointWriteField, rule.StoryPointWriteFieldSubtask, rule.EditedBy, rule.ProjectKey)
	return err
}

func (r *Repository) SetActive(ctx context.Context, projectKey string, active bool, editedBy string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ms_tracking_rule SET isActive = ?, editedBy = ?, editedDate = CURRENT_TIMESTAMP WHERE projectKey = ?`,
		active, editedBy, projectKey)
	return err
}
