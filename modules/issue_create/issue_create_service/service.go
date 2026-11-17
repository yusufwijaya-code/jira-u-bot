// Package issue_create_service creates Jira cards from Papan Tiket (Tech
// Spec "Papan Tiket Revamp & Buat Card Jira", Phase 2): createmeta-driven
// forms, Gemini-normalized Task Breakdowns and sequential bulk subtasks,
// always with the caller's own Jira token.
package issue_create_service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_client_service"
	"nudge/modules/user_dashboard/user_dashboard_dto"
)

var (
	ErrTokenRequired   = errors.New("issue_create: jira token required")
	ErrBoardNotAllowed = errors.New("issue_create: board not registered for user")
	ErrParentNotFound  = errors.New("issue_create: parent not found")
	ErrParentInvalid   = errors.New("issue_create: parent invalid")
	ErrAIUnavailable   = errors.New("issue_create: ai not configured")
)

// ParentInvalidError says why a ticket can't be a subtask parent; it
// matches ErrParentInvalid under errors.Is.
type ParentInvalidError struct{ Message string }

func (e *ParentInvalidError) Error() string        { return "issue_create: parent invalid: " + e.Message }
func (e *ParentInvalidError) Is(target error) bool { return target == ErrParentInvalid }

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

type RateLimitError struct {
	Scope             string
	RetryAfterSeconds int
}

func (e *RateLimitError) Error() string { return "issue_create: rate limited (" + e.Scope + ")" }

type Credentials interface {
	ResolveCredential(ctx context.Context, email string) (*jira_client_dto.Credential, error)
}
type Boards interface {
	ListBoards(ctx context.Context, email string) ([]user_dashboard_dto.BoardSummary, error)
}
type AI interface {
	Configured() bool
	GenerateJSON(ctx context.Context, system, user string, schema map[string]any, out any) error
}

type Deps struct {
	Jira        jira_client_service.IssueCreateClient
	Credentials Credentials
	Boards      Boards
	AI          AI
	Now         func() time.Time
}

type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Field struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Multi    bool     `json:"multi"`
	Required bool     `json:"required"`
	Options  []Option `json:"options,omitempty"`
}
type TypeMeta struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Subtask         bool    `json:"subtask"`
	Supported       bool    `json:"supported"`
	ParentAllowed   bool    `json:"parentAllowed"`
	StoryPointField string  `json:"storyPointField,omitempty"`
	Fields          []Field `json:"fields"`
}
type Meta struct {
	ProjectKey    string     `json:"projectKey"`
	Types         []TypeMeta `json:"types"`
	SubtaskTypeID string     `json:"subtaskTypeId"`
}
type User struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}
type Brief struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
}
type Parent struct {
	Key            string  `json:"key"`
	Summary        string  `json:"summary"`
	Type           string  `json:"type"`
	TypeIconURL    string  `json:"typeIconUrl"`
	Status         string  `json:"status"`
	StatusCategory string  `json:"statusCategory"`
	Subtasks       []Brief `json:"subtasks"`
}

// Candidate is one pickable parent in the Buat Card parent picker.
type Candidate struct {
	Key            string `json:"key"`
	Summary        string `json:"summary"`
	Type           string `json:"type"`
	TypeIconURL    string `json:"typeIconUrl"`
	Status         string `json:"status"`
	StatusCategory string `json:"statusCategory"`
	SubtaskCount   int    `json:"subtaskCount"`
}
type CreateRequest struct {
	BoardID           int64          `json:"boardId"`
	TypeID            string         `json:"typeId"`
	Summary           string         `json:"summary"`
	Description       string         `json:"description"`
	StoryPoints       *float64       `json:"storyPoints"`
	AssigneeAccountID *string        `json:"assigneeAccountId"`
	ParentKey         *string        `json:"parentKey"`
	Fields            map[string]any `json:"fields"`
}
type Row struct {
	Title       string   `json:"title"`
	StoryPoints *float64 `json:"storyPoints"`
	Details     string   `json:"details"`
}
type Normalized struct {
	Rows     []Row    `json:"rows"`
	Warnings []string `json:"warnings"`
	Cached   bool     `json:"cached"`
}
type BulkRow struct {
	ClientID          string   `json:"clientId"`
	Title             string   `json:"title"`
	StoryPoints       *float64 `json:"storyPoints"`
	AssigneeAccountID *string  `json:"assigneeAccountId"`
	Details           string   `json:"details"`
}
type BulkResult struct {
	ClientID string `json:"clientId"`
	Key      string `json:"key,omitempty"`
	Error    string `json:"error,omitempty"`
}
type Status struct {
	TokenReady bool `json:"tokenReady"`
	AIReady    bool `json:"aiReady"`
}

const (
	maxSummaryRunes = 255
	maxBulkRows     = 50
	maxCandidates   = 50
)

type Service struct {
	d       Deps
	mu      sync.Mutex
	meta    map[string]cachedMeta       // projectKey → meta
	results map[string]cachedNormalized // projectKey|sha256 → rows
	limiter *limiter
}

type cachedMeta struct {
	meta Meta
	at   time.Time
	// rule is the project's tracking rule the meta was mapped with; a rule
	// change (Aturan Board) makes the entry stale.
	rule jira_client_dto.TrackingRule
}

func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d, meta: map[string]cachedMeta{}, results: map[string]cachedNormalized{}, limiter: newLimiter(3, 12, time.Minute)}
}

func (s *Service) cred(ctx context.Context, email string) (*jira_client_dto.Credential, error) {
	cred, err := s.d.Credentials.ResolveCredential(ctx, email)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, ErrTokenRequired
	}
	return cred, nil
}

func (s *Service) projectOf(ctx context.Context, email string, boardID int64) (string, error) {
	boards, err := s.d.Boards.ListBoards(ctx, email)
	if err != nil {
		return "", err
	}
	for _, b := range boards {
		if b.BoardID == boardID && b.ProjectKey != "" {
			return b.ProjectKey, nil
		}
	}
	return "", ErrBoardNotAllowed
}

func (s *Service) Status(ctx context.Context, email string) (Status, error) {
	cred, err := s.d.Credentials.ResolveCredential(ctx, email)
	if err != nil {
		return Status{}, err
	}
	return Status{TokenReady: cred != nil, AIReady: s.d.AI != nil && s.d.AI.Configured()}, nil
}

func (s *Service) Meta(ctx context.Context, email string, boardID int64) (Meta, error) {
	cred, err := s.cred(ctx, email)
	if err != nil {
		return Meta{}, err
	}
	project, err := s.projectOf(ctx, email, boardID)
	if err != nil {
		return Meta{}, err
	}
	return s.metaFor(ctx, cred, project)
}

func (s *Service) metaFor(ctx context.Context, cred *jira_client_dto.Credential, project string) (Meta, error) {
	rule := jira_client_dto.RuleFor(project)
	s.mu.Lock()
	c, ok := s.meta[project]
	s.mu.Unlock()
	if ok && s.d.Now().Sub(c.at) < time.Hour && reflect.DeepEqual(c.rule, rule) {
		return c.meta, nil
	}
	types, err := s.d.Jira.IssueTypesAs(ctx, cred, project)
	if err != nil {
		return Meta{}, err
	}
	m := Meta{ProjectKey: project, Types: []TypeMeta{}}
	for _, t := range types {
		fields, err := s.d.Jira.IssueTypeFieldsAs(ctx, cred, project, t.ID)
		if err != nil {
			return Meta{}, err
		}
		tm := mapType(rule, t, fields)
		m.Types = append(m.Types, tm)
		if t.Subtask && m.SubtaskTypeID == "" {
			m.SubtaskTypeID = t.ID
		}
	}
	s.mu.Lock()
	s.meta[project] = cachedMeta{meta: m, at: s.d.Now(), rule: rule}
	s.mu.Unlock()
	return m, nil
}

func (s *Service) Users(ctx context.Context, email string, boardID int64, q string) ([]User, error) {
	if len([]rune(strings.TrimSpace(q))) < 2 {
		return []User{}, nil
	}
	cred, err := s.cred(ctx, email)
	if err != nil {
		return nil, err
	}
	project, err := s.projectOf(ctx, email, boardID)
	if err != nil {
		return nil, err
	}
	found, err := s.d.Jira.AssignableUsersAs(ctx, cred, project, strings.TrimSpace(q))
	if err != nil {
		return nil, err
	}
	out := []User{}
	for _, u := range found {
		if !u.Active {
			continue
		}
		out = append(out, User{AccountID: u.AccountID, DisplayName: u.DisplayName, AvatarURL: u.AvatarURLs["24x24"]})
	}
	return out, nil
}

func (s *Service) Parent(ctx context.Context, email string, boardID int64, key string) (Parent, error) {
	cred, err := s.cred(ctx, email)
	if err != nil {
		return Parent{}, err
	}
	project, err := s.projectOf(ctx, email, boardID)
	if err != nil {
		return Parent{}, err
	}
	return s.parentFor(ctx, cred, project, strings.ToUpper(strings.TrimSpace(key)))
}

func (s *Service) parentFor(ctx context.Context, cred *jira_client_dto.Credential, project, key string) (Parent, error) {
	brief, err := s.d.Jira.IssueBriefAs(ctx, cred, key)
	if errors.Is(err, jira_client_service.ErrIssueNotFound) {
		return Parent{}, ErrParentNotFound
	}
	if err != nil {
		return Parent{}, err
	}
	if err := parentProblem(brief, project); err != nil {
		return Parent{}, err
	}
	f := brief.Fields
	p := Parent{Key: brief.Key, Summary: f.Summary, Type: f.IssueType.Name, TypeIconURL: f.IssueType.IconURL,
		Status: f.Status.Name, StatusCategory: f.Status.StatusCategory.Key, Subtasks: []Brief{}}
	for _, st := range brief.Fields.Subtasks {
		p.Subtasks = append(p.Subtasks, Brief{Key: st.Key, Summary: st.Fields.Summary})
	}
	return p, nil
}

// parentProblem: only standard tickets (hierarchyLevel 0) of the board's own
// project can hold subtasks — not subtasks (-1), not epics (1).
func parentProblem(b jira_client_dto.IssueBrief, project string) error {
	t := b.Fields.IssueType
	name := t.Name
	if name == "" {
		name = "Tiket ini"
	}
	switch {
	case !strings.EqualFold(b.Fields.Project.Key, project):
		return &ParentInvalidError{Message: "Tiket dari project lain tidak bisa jadi parent"}
	case t.Subtask || t.HierarchyLevel < 0:
		return &ParentInvalidError{Message: name + " adalah subtask, tidak bisa punya subtask"}
	case t.HierarchyLevel != 0:
		return &ParentInvalidError{Message: name + " tidak bisa punya subtask"}
	}
	return nil
}

// Parents lists parent candidates: the board project's tickets in open
// sprints (non-subtask, non-epic), optionally narrowed by q (a key or a
// summary prefix), newest update first.
func (s *Service) Parents(ctx context.Context, email string, boardID int64, q string) ([]Candidate, error) {
	cred, err := s.cred(ctx, email)
	if err != nil {
		return nil, err
	}
	project, err := s.projectOf(ctx, email, boardID)
	if err != nil {
		return nil, err
	}
	found, err := s.d.Jira.SearchBriefsAs(ctx, cred, parentsJQL(project, q), maxCandidates)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, b := range found {
		f := b.Fields
		if f.IssueType.Subtask || f.IssueType.HierarchyLevel != 0 {
			continue
		}
		out = append(out, Candidate{Key: b.Key, Summary: f.Summary, Type: f.IssueType.Name, TypeIconURL: f.IssueType.IconURL,
			Status: f.Status.Name, StatusCategory: f.Status.StatusCategory.Key, SubtaskCount: len(f.Subtasks)})
	}
	return out, nil
}

var issueKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-[0-9]+$`)

// luceneSpecial are the characters Jira text search treats as operators;
// they're dropped from a summary query rather than escaped.
var luceneSpecial = strings.NewReplacer(
	`"`, " ", `\`, " ", "'", " ", "+", " ", "-", " ", "&", " ", "|", " ", "!", " ", "(", " ", ")", " ",
	"{", " ", "}", " ", "[", " ", "]", " ", "^", " ", "~", " ", "*", " ", "?", " ", ":", " ", "/", " ",
)

func parentsJQL(project, q string) string {
	// The search runs with the caller's own token, so currentUser() is them:
	// only their own sprint tickets are offered (any other key can still be typed).
	jql := fmt.Sprintf(`project = "%s" AND sprint in openSprints() AND assignee = currentUser() AND issuetype not in subTaskIssueTypes()`, jqlQuote(project))
	q = strings.TrimSpace(q)
	if key := strings.ToUpper(q); issueKeyRe.MatchString(key) {
		jql += fmt.Sprintf(` AND key = "%s"`, key)
	} else if text := strings.Join(strings.Fields(luceneSpecial.Replace(q)), " "); text != "" {
		jql += fmt.Sprintf(` AND summary ~ "%s*"`, jqlQuote(text))
	}
	return jql + " ORDER BY updated DESC"
}

// jqlQuote escapes a value for a double-quoted JQL string.
func jqlQuote(v string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v)
}

func (s *Service) CreateOne(ctx context.Context, email string, req CreateRequest) (string, error) {
	summary := strings.TrimSpace(req.Summary)
	if summary == "" || len([]rune(summary)) > maxSummaryRunes {
		return "", &ValidationError{Message: "Judul wajib diisi (maks 255 karakter)"}
	}
	if err := validSP(req.StoryPoints); err != nil {
		return "", err
	}
	cred, err := s.cred(ctx, email)
	if err != nil {
		return "", err
	}
	project, err := s.projectOf(ctx, email, req.BoardID)
	if err != nil {
		return "", err
	}
	meta, err := s.metaFor(ctx, cred, project)
	if err != nil {
		return "", err
	}
	t, ok := meta.typeByID(req.TypeID)
	if !ok || !t.Supported {
		return "", &ValidationError{Message: "Tipe card tidak dikenal atau tidak didukung"}
	}
	fields := map[string]any{
		"project":   map[string]any{"key": project},
		"issuetype": map[string]any{"id": t.ID},
		"summary":   summary,
	}
	if d := strings.TrimSpace(req.Description); d != "" {
		fields["description"] = adf(d)
	}
	if req.StoryPoints != nil && t.StoryPointField != "" {
		fields[t.StoryPointField] = *req.StoryPoints
	}
	if req.ParentKey != nil && strings.TrimSpace(*req.ParentKey) != "" {
		if _, err := s.parentFor(ctx, cred, project, strings.ToUpper(strings.TrimSpace(*req.ParentKey))); err != nil {
			return "", err
		}
		fields["parent"] = map[string]any{"key": strings.ToUpper(strings.TrimSpace(*req.ParentKey))}
	} else if t.Subtask {
		return "", &ValidationError{Message: "Parent wajib diisi untuk subtask"}
	}
	for _, f := range t.Fields {
		v, ok := encodeField(f, req.Fields[f.ID])
		if !ok {
			if f.Required {
				return "", &ValidationError{Message: f.Name + " wajib diisi"}
			}
			continue
		}
		fields[f.ID] = v
	}
	assignee, err := s.assignee(ctx, cred, req.AssigneeAccountID)
	if err != nil {
		return "", err
	}
	fields["assignee"] = map[string]any{"accountId": assignee}
	return s.d.Jira.CreateIssueAs(ctx, cred, fields)
}

func (s *Service) assignee(ctx context.Context, cred *jira_client_dto.Credential, id *string) (string, error) {
	if id != nil && strings.TrimSpace(*id) != "" {
		return strings.TrimSpace(*id), nil
	}
	return s.d.Jira.MyselfAs(ctx, cred)
}

func (s *Service) CreateSubtasks(ctx context.Context, email string, boardID int64, parentKey string, rows []BulkRow) ([]BulkResult, error) {
	if len(rows) == 0 || len(rows) > maxBulkRows {
		return nil, &ValidationError{Message: fmt.Sprintf("Jumlah baris harus 1–%d", maxBulkRows)}
	}
	for _, r := range rows {
		if strings.TrimSpace(r.Title) == "" {
			return nil, &ValidationError{Message: "Judul tidak boleh kosong"}
		}
	}
	cred, err := s.cred(ctx, email)
	if err != nil {
		return nil, err
	}
	project, err := s.projectOf(ctx, email, boardID)
	if err != nil {
		return nil, err
	}
	parent, err := s.parentFor(ctx, cred, project, strings.ToUpper(strings.TrimSpace(parentKey)))
	if err != nil {
		return nil, err
	}
	meta, err := s.metaFor(ctx, cred, project)
	if err != nil {
		return nil, err
	}
	sub, ok := meta.typeByID(meta.SubtaskTypeID)
	if !ok {
		return nil, &ValidationError{Message: "Board ini tidak punya tipe subtask"}
	}
	me, err := s.d.Jira.MyselfAs(ctx, cred)
	if err != nil {
		return nil, err
	}
	results := make([]BulkResult, 0, len(rows))
	for _, r := range rows {
		res := BulkResult{ClientID: r.ClientID}
		if err := validSP(r.StoryPoints); err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		fields := map[string]any{
			"project":   map[string]any{"key": project},
			"issuetype": map[string]any{"id": sub.ID},
			"parent":    map[string]any{"key": parent.Key},
			"summary":   truncateRunes(strings.TrimSpace(r.Title), maxSummaryRunes),
			"assignee":  map[string]any{"accountId": me},
		}
		if r.AssigneeAccountID != nil && strings.TrimSpace(*r.AssigneeAccountID) != "" {
			fields["assignee"] = map[string]any{"accountId": strings.TrimSpace(*r.AssigneeAccountID)}
		}
		if r.StoryPoints != nil && sub.StoryPointField != "" {
			fields[sub.StoryPointField] = *r.StoryPoints
		}
		if d := strings.TrimSpace(r.Details); d != "" {
			fields["description"] = adf(d)
		}
		key, err := s.d.Jira.CreateIssueAs(ctx, cred, fields)
		if err != nil {
			res.Error = friendlyJiraError(err)
		} else {
			res.Key = key
		}
		results = append(results, res)
	}
	return results, nil
}

func validSP(sp *float64) error {
	if sp != nil && (*sp < 0 || *sp > 100 || *sp != *sp) {
		return &ValidationError{Message: "SP harus 0–100"}
	}
	return nil
}

func friendlyJiraError(err error) string {
	var rej *jira_client_service.RejectedError
	if errors.As(err, &rej) {
		return strings.TrimPrefix(rej.Error(), "jira rejected the issue: ")
	}
	log.Printf("issue_create: jira error: %v", err)
	return "Jira tidak merespons, coba lagi"
}
