package tracking_rule

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"nudge/modules/board/board_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_client_service"
)

const (
	// RefreshInterval: rules changed outside this process (another
	// instance, a manual DB edit) apply within this window (AC5.4).
	RefreshInterval = 5 * time.Minute
	lookupTTL       = 10 * time.Minute
	boardStatusTTL  = time.Hour
)

type Store interface {
	List(ctx context.Context) ([]Rule, error)
	Find(ctx context.Context, projectKey string) (Rule, bool, error)
	Insert(ctx context.Context, rule Rule) error
	Update(ctx context.Context, rule Rule) error
	// SetActive returns ErrLastActiveRule, atomically, when it would switch
	// off the last active rule.
	SetActive(ctx context.Context, projectKey string, active bool, editedBy string) error
}

// Recalculator is sprint_backfill_service's "every user" run.
type Recalculator interface {
	RunAll(ctx context.Context) int
}

// BoardLookup is the cached ms_board read of board_service.
type BoardLookup interface {
	Get(ctx context.Context, boardID int64) (board_dto.Board, bool, error)
}

type Deps struct {
	Store  Store
	Jira   jira_client_service.ProjectMetaClient
	Boards BoardLookup
	Recalc Recalculator
	// Apply installs the active rules; defaults to jira_client_dto.SetRules.
	Apply func([]jira_client_dto.TrackingRule)
	Now   func() time.Time
}

type cachedLookup struct {
	lookup Lookup
	at     time.Time
}

type cachedFields struct {
	list []jira_client_dto.NumberField
	at   time.Time
}

type cachedStatuses struct {
	list []string
	at   time.Time
}

type Service struct {
	d  Deps
	mu sync.Mutex
	// reloadMu serializes Reload: a ticker List read before a save can't
	// install its stale ruleset after the save's Reload.
	reloadMu      sync.Mutex
	lookups       map[string]cachedLookup
	fields        *cachedFields
	boardStatuses map[string]cachedStatuses
	recalcRunning atomic.Bool
}

func NewService(d Deps) *Service {
	if d.Apply == nil {
		d.Apply = jira_client_dto.SetRules
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d, lookups: map[string]cachedLookup{}, boardStatuses: map[string]cachedStatuses{}}
}

// Reload loads the table and installs the active rules. On error the
// registry keeps its last rules (AC5.6).
func (s *Service) Reload(ctx context.Context) error {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	rules, err := s.d.Store.List(ctx)
	if err != nil {
		return fmt.Errorf("tracking_rule: reload: %w", err)
	}
	active := make([]jira_client_dto.TrackingRule, 0, len(rules))
	for _, r := range rules {
		if r.IsActive {
			active = append(active, r.Tracking())
		}
	}
	if len(active) == 0 {
		log.Printf("tracking_rule: WARN no active tracking rule — the poller searches nothing")
	}
	s.d.Apply(active)
	return nil
}

// RunRefresh reloads every RefreshInterval until ctx ends.
func (s *Service) RunRefresh(ctx context.Context) {
	ticker := time.NewTicker(RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Reload(ctx); err != nil {
				log.Printf("tracking_rule: ERROR refresh failed, keeping the last rules: %v", err)
			}
		}
	}
}

func jiraErr(err error) error {
	if errors.Is(err, jira_client_service.ErrProjectNotFound) {
		return ErrProjectNotFound
	}
	log.Printf("tracking_rule: jira lookup failed: %v", err)
	return ErrJiraUnavailable
}

func (s *Service) numberFields(ctx context.Context) ([]jira_client_dto.NumberField, error) {
	s.mu.Lock()
	cached := s.fields
	s.mu.Unlock()
	if cached != nil && s.d.Now().Sub(cached.at) < lookupTTL {
		return cached.list, nil
	}
	list, err := s.d.Jira.GetNumberFields(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.fields = &cachedFields{list: list, at: s.d.Now()}
	s.mu.Unlock()
	return list, nil
}

// Lookup is the project's Jira data for the form (AC3.1), cached 10 minutes.
func (s *Service) Lookup(ctx context.Context, projectKey string) (Lookup, error) {
	key, ok := NormalizeProjectKey(projectKey)
	if !ok {
		return Lookup{}, ErrInvalidProjectKey
	}
	s.mu.Lock()
	cached, hit := s.lookups[key]
	s.mu.Unlock()
	if hit && s.d.Now().Sub(cached.at) < lookupTTL {
		return cached.lookup, nil
	}
	project, err := s.d.Jira.GetProject(ctx, key)
	if err != nil {
		return Lookup{}, jiraErr(err)
	}
	statuses, err := s.d.Jira.GetProjectStatuses(ctx, key)
	if err != nil {
		return Lookup{}, jiraErr(err)
	}
	fields, err := s.numberFields(ctx)
	if err != nil {
		return Lookup{}, jiraErr(err)
	}
	l := Lookup{ProjectKey: key, ProjectName: project.Name, AvatarURL: project.AvatarURL(), Statuses: statuses, NumberFields: fields}
	s.mu.Lock()
	s.lookups[key] = cachedLookup{lookup: l, at: s.d.Now()}
	s.mu.Unlock()
	return l, nil
}

// fieldNames maps field ID → Jira name; empty when Jira is down (views then
// show the ID as the name).
func (s *Service) fieldNames(ctx context.Context) map[string]string {
	names := map[string]string{}
	fields, err := s.numberFields(ctx)
	if err != nil {
		log.Printf("tracking_rule: field names unavailable: %v", err)
		return names
	}
	for _, f := range fields {
		names[f.ID] = f.Name
	}
	return names
}

func view(r Rule, names map[string]string) RuleView {
	v := RuleView{
		ProjectKey: r.ProjectKey, ProjectName: r.ProjectName, TrackedStatuses: r.TrackedStatuses, DoneStatus: r.DoneStatus,
		StoryPointFields: make([]FieldRef, 0, len(r.StoryPointFields)), StoryPointWriteField: r.StoryPointWriteField,
		StoryPointWriteFieldSubtask: r.StoryPointWriteFieldSubtask, IsActive: r.IsActive, EditedBy: r.EditedBy, EditedDate: r.EditedDate,
	}
	if r.ReviewStatus != "" {
		review := r.ReviewStatus
		v.ReviewStatus = &review
	}
	for _, id := range r.StoryPointFields {
		name := names[id]
		if name == "" {
			name = id
		}
		v.StoryPointFields = append(v.StoryPointFields, FieldRef{ID: id, Name: name})
	}
	return v
}

func (s *Service) List(ctx context.Context) ([]RuleView, error) {
	rules, err := s.d.Store.List(ctx)
	if err != nil {
		return nil, err
	}
	names := s.fieldNames(ctx)
	out := make([]RuleView, 0, len(rules))
	for _, r := range rules {
		out = append(out, view(r, names))
	}
	return out, nil
}

func (s *Service) Create(ctx context.Context, in RuleInput, editor string) (RuleView, error) {
	key, ok := NormalizeProjectKey(in.ProjectKey)
	if !ok {
		return RuleView{}, &ValidationError{Fields: map[string]string{"projectKey": "Project key: huruf besar, angka atau _, 2–20 karakter, diawali huruf"}}
	}
	if _, found, err := s.d.Store.Find(ctx, key); err != nil {
		return RuleView{}, err
	} else if found {
		return RuleView{}, ErrRuleExists
	}
	l, err := s.Lookup(ctx, key)
	if err != nil {
		return RuleView{}, err
	}
	rule, errs := buildRule(in, l)
	if len(errs) > 0 {
		return RuleView{}, &ValidationError{Fields: errs}
	}
	rule.ProjectKey, rule.ProjectName, rule.IsActive, rule.EditedBy = key, l.ProjectName, true, editor
	if err := s.d.Store.Insert(ctx, rule); err != nil {
		return RuleView{}, err
	}
	return s.afterSave(ctx, key)
}

func (s *Service) Update(ctx context.Context, projectKey string, in RuleInput, editor string) (RuleView, error) {
	key, ok := NormalizeProjectKey(projectKey)
	if !ok {
		return RuleView{}, ErrRuleNotFound
	}
	existing, found, err := s.d.Store.Find(ctx, key)
	if err != nil {
		return RuleView{}, err
	}
	if !found {
		return RuleView{}, ErrRuleNotFound
	}
	l, err := s.Lookup(ctx, key)
	if err != nil {
		return RuleView{}, err
	}
	rule, errs := buildRule(in, l)
	if len(errs) > 0 {
		return RuleView{}, &ValidationError{Fields: errs}
	}
	rule.ProjectKey, rule.ProjectName, rule.IsActive, rule.EditedBy = key, l.ProjectName, existing.IsActive, editor
	if rule.ProjectName == "" {
		rule.ProjectName = existing.ProjectName
	}
	if err := s.d.Store.Update(ctx, rule); err != nil {
		return RuleView{}, err
	}
	return s.afterSave(ctx, key)
}

// SetActive switches a rule on/off ("hapus" = nonaktifkan). The last active
// rule can't be switched off (AC4.3): the store checks it atomically.
func (s *Service) SetActive(ctx context.Context, projectKey string, active bool, editor string) (RuleView, error) {
	key, ok := NormalizeProjectKey(projectKey)
	if !ok {
		return RuleView{}, ErrRuleNotFound
	}
	if _, found, err := s.d.Store.Find(ctx, key); err != nil {
		return RuleView{}, err
	} else if !found {
		return RuleView{}, ErrRuleNotFound
	}
	if err := s.d.Store.SetActive(ctx, key, active, editor); err != nil {
		return RuleView{}, err
	}
	return s.afterSave(ctx, key)
}

// afterSave refreshes the registry in the same request (AC3.4/AC5.4) and
// returns the stored rule. A failed reload is logged, not returned: the row
// is saved and the 5-minute refresh retries.
func (s *Service) afterSave(ctx context.Context, key string) (RuleView, error) {
	// The row is saved: a client disconnect must not skip the reload.
	if err := s.Reload(context.WithoutCancel(ctx)); err != nil {
		log.Printf("tracking_rule: ERROR reload after saving %s failed (the periodic refresh retries): %v", key, err)
	}
	saved, found, err := s.d.Store.Find(ctx, key)
	if err != nil {
		return RuleView{}, err
	}
	if !found {
		return RuleView{}, ErrRuleNotFound
	}
	return view(saved, s.fieldNames(ctx)), nil
}

// Recalculate starts sprint_backfill RunAll in the background; one run at a time.
func (s *Service) Recalculate() error {
	if !s.recalcRunning.CompareAndSwap(false, true) {
		return ErrAlreadyRunning
	}
	go func() {
		defer s.recalcRunning.Store(false)
		added := s.d.Recalc.RunAll(context.Background())
		log.Printf("tracking_rule: recalculation for every user done (%d session(s) added)", added)
	}()
	return nil
}

// BoardStatuses is the workflow of the board's project (Pengaturan, AC5.5),
// cached an hour per project.
func (s *Service) BoardStatuses(ctx context.Context, boardID int64) ([]string, error) {
	board, found, err := s.d.Boards.Get(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if !found || board.ProjectKey == "" {
		return nil, ErrBoardNotFound
	}
	s.mu.Lock()
	cached, hit := s.boardStatuses[board.ProjectKey]
	s.mu.Unlock()
	if hit && s.d.Now().Sub(cached.at) < boardStatusTTL {
		return cached.list, nil
	}
	list, err := s.d.Jira.GetProjectStatuses(ctx, board.ProjectKey)
	if err != nil {
		log.Printf("tracking_rule: statuses of board %d (%s) failed: %v", boardID, board.ProjectKey, err)
		return nil, ErrJiraUnavailable
	}
	s.mu.Lock()
	s.boardStatuses[board.ProjectKey] = cachedStatuses{list: list, at: s.d.Now()}
	s.mu.Unlock()
	return list, nil
}
