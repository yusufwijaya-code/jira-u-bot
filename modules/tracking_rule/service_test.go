package tracking_rule

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nudge/modules/board/board_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_client_service"
)

var ctx = context.Background()

type fakeStore struct {
	rules     map[string]Rule
	listErr   error
	listCalls atomic.Int32
	// afterList, when set, runs once after the first List read its rows.
	afterList func()
}

func newFakeStore(rules ...Rule) *fakeStore {
	s := &fakeStore{rules: map[string]Rule{}}
	for _, r := range rules {
		s.rules[r.ProjectKey] = r
	}
	return s
}

func (s *fakeStore) List(ctx context.Context) ([]Rule, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]Rule, 0, len(s.rules))
	for _, r := range s.rules {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProjectKey < out[j].ProjectKey })
	if s.listCalls.Add(1) == 1 && s.afterList != nil {
		s.afterList()
	}
	return out, nil
}

func (s *fakeStore) Find(_ context.Context, key string) (Rule, bool, error) {
	r, ok := s.rules[key]
	return r, ok, nil
}

func (s *fakeStore) Insert(_ context.Context, r Rule) error {
	if _, ok := s.rules[r.ProjectKey]; ok {
		return ErrRuleExists
	}
	s.rules[r.ProjectKey] = r
	return nil
}

func (s *fakeStore) Update(_ context.Context, r Rule) error {
	s.rules[r.ProjectKey] = r
	return nil
}

func (s *fakeStore) SetActive(_ context.Context, key string, active bool, by string) error {
	if !active {
		var others int
		for k, r := range s.rules {
			if r.IsActive && k != key {
				others++
			}
		}
		if s.rules[key].IsActive && others == 0 {
			return ErrLastActiveRule
		}
	}
	r := s.rules[key]
	r.IsActive, r.EditedBy = active, by
	s.rules[key] = r
	return nil
}

type fakeJira struct {
	projects map[string]jira_client_dto.ProjectInfo
	statuses map[string][]string
	fields   []jira_client_dto.NumberField
	err      error
	calls    int
}

func (f *fakeJira) GetProject(_ context.Context, key string) (jira_client_dto.ProjectInfo, error) {
	f.calls++
	if f.err != nil {
		return jira_client_dto.ProjectInfo{}, f.err
	}
	p, ok := f.projects[key]
	if !ok {
		return jira_client_dto.ProjectInfo{}, jira_client_service.ErrProjectNotFound
	}
	return p, nil
}

func (f *fakeJira) GetProjectStatuses(_ context.Context, key string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	s, ok := f.statuses[key]
	if !ok {
		return nil, jira_client_service.ErrProjectNotFound
	}
	return s, nil
}

func (f *fakeJira) GetNumberFields(context.Context) ([]jira_client_dto.NumberField, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.fields, nil
}

func newFakeJira() *fakeJira {
	return &fakeJira{
		projects: map[string]jira_client_dto.ProjectInfo{"POS": {Key: "POS", Name: "ESB POS"}, "CORE": {Key: "CORE", Name: "ESB Core"}, "OMS": {Key: "OMS", Name: "ESB OMS"}},
		statuses: map[string][]string{
			"POS":  {"To Do", "In Progress", "Code Review", "Done"},
			"CORE": {"To Do", "Code Process", "CODE REVIEW", "Code Completed", "Done"},
			"OMS":  {"To Do", "Code Process", "Code Review", "Done"},
		},
		fields: []jira_client_dto.NumberField{
			{ID: "customfield_10016", Name: "Story point estimate", StoryPointLike: true},
			{ID: "customfield_10125", Name: "Story Point Estimate", StoryPointLike: true},
			{ID: "customfield_11053", Name: "Story Points", StoryPointLike: true},
			{ID: "customfield_200", Name: "Effort"},
		},
	}
}

type fakeRecalc struct {
	started, release chan struct{}
	// stopped receives the run's ctx error when it ends.
	stopped chan error
}

func (f *fakeRecalc) RunAll(ctx context.Context) int {
	if f.started != nil {
		f.started <- struct{}{}
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
		}
	}
	if f.stopped != nil {
		f.stopped <- ctx.Err()
	}
	return 0
}

type fakeBoards map[int64]board_dto.Board

func (f fakeBoards) Get(_ context.Context, id int64) (board_dto.Board, bool, error) {
	b, ok := f[id]
	return b, ok, nil
}

// seedRules are OMS & CORE as the 0021b seed (= the builtin rules).
func seedRules() []Rule {
	var out []Rule
	for _, t := range jira_client_dto.BuiltinRules() {
		out = append(out, Rule{
			ProjectKey: t.ProjectKey, ProjectName: "ESB " + t.ProjectKey, TrackedStatuses: t.TrackedStatuses, DoneStatus: t.DoneStatus,
			ReviewStatus: t.ReviewStatus, StoryPointFields: t.StoryPointFields, StoryPointWriteField: t.StoryPointWriteField,
			StoryPointWriteFieldSubtask: t.StoryPointWriteFieldSubtask, IsActive: true, EditedBy: "seed",
		})
	}
	return out // [CORE, OMS]
}

type harness struct {
	svc     *Service
	store   *fakeStore
	jira    *fakeJira
	applied [][]jira_client_dto.TrackingRule
	now     time.Time
}

func newHarness(rules ...Rule) *harness {
	h := &harness{store: newFakeStore(rules...), jira: newFakeJira(), now: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
	h.svc = NewService(Deps{
		Store: h.store, Jira: h.jira, Recalc: &fakeRecalc{},
		Boards: fakeBoards{26: {BoardID: 26, ProjectKey: "CORE"}, 99: {BoardID: 99}},
		Apply:  func(r []jira_client_dto.TrackingRule) { h.applied = append(h.applied, r) },
		Now:    func() time.Time { return h.now },
	})
	return h
}

func (h *harness) lastAppliedKeys() []string {
	if len(h.applied) == 0 {
		return nil
	}
	keys := []string{}
	for _, r := range h.applied[len(h.applied)-1] {
		keys = append(keys, r.ProjectKey)
	}
	return keys
}

func validPOS() RuleInput {
	review := "Code Review"
	return RuleInput{
		ProjectKey: " pos ", TrackedStatuses: []string{"in progress", "Code Review"}, DoneStatus: "done", ReviewStatus: &review,
		StoryPointFields: []string{"customfield_10016", "customfield_200"}, StoryPointWriteField: "customfield_10016", StoryPointWriteFieldSubtask: "customfield_200",
	}
}

func TestReload_AppliesActiveRulesAndKeepsTheLastOnError(t *testing.T) {
	rules := seedRules()
	rules[1].IsActive = false // OMS
	h := newHarness(rules...)
	if err := h.svc.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if got := h.lastAppliedKeys(); !reflect.DeepEqual(got, []string{"CORE"}) {
		t.Fatalf("applied = %v, want only the active CORE", got)
	}
	if !reflect.DeepEqual(h.applied[0][0], jira_client_dto.BuiltinRules()[0]) {
		t.Errorf("CORE = %+v, want the builtin rule", h.applied[0][0])
	}
	h.store.listErr = errors.New("db down")
	if err := h.svc.Reload(ctx); err == nil {
		t.Fatal("a failed reload must return its error")
	}
	if len(h.applied) != 1 {
		t.Error("a failed reload must not touch the registry (AC5.6)")
	}
}

func TestReload_SerializedSoAStaleReadCannotWinOverASave(t *testing.T) {
	h := newHarness(seedRules()...)
	read, release := make(chan struct{}), make(chan struct{})
	h.store.afterList = func() { close(read); <-release }
	var mu sync.Mutex
	h.svc.d.Apply = func(r []jira_client_dto.TrackingRule) { mu.Lock(); h.applied = append(h.applied, r); mu.Unlock() }
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); h.svc.Reload(ctx) }() // the ticker: reads [CORE, OMS], then stalls
	<-read
	pos := seedRules()[0]
	pos.ProjectKey = "POS"
	h.store.rules["POS"] = pos // the save
	go func() { defer wg.Done(); h.svc.Reload(ctx) }()
	time.Sleep(50 * time.Millisecond)
	if n := h.store.listCalls.Load(); n != 1 {
		t.Errorf("list calls = %d while a reload runs, want 1 (serialized)", n)
	}
	close(release)
	wg.Wait()
	if keys := h.lastAppliedKeys(); !reflect.DeepEqual(keys, []string{"CORE", "OMS", "POS"}) {
		t.Fatalf("registry = %v, want the save's ruleset last", keys)
	}
}

func TestCreate_ReloadsEvenWhenTheClientDisconnected(t *testing.T) {
	h := newHarness(seedRules()...)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := h.svc.Create(cancelled, validPOS(), "admin@esb.co.id"); err != nil {
		t.Fatal(err)
	}
	if keys := h.lastAppliedKeys(); !reflect.DeepEqual(keys, []string{"CORE", "OMS", "POS"}) {
		t.Fatalf("registry = %v, want POS applied despite the cancelled request", keys)
	}
}

func TestReload_NoActiveRuleAppliesAnEmptySet(t *testing.T) {
	h := newHarness()
	if err := h.svc.Reload(ctx); err != nil || len(h.applied) != 1 || len(h.applied[0]) != 0 {
		t.Fatalf("err=%v applied=%v, want one empty set (AC1.4)", err, h.applied)
	}
}

func TestLookup_NormalizesCachesAndMapsJiraErrors(t *testing.T) {
	h := newHarness()
	l, err := h.svc.Lookup(ctx, " pos ", false)
	if err != nil || l.ProjectKey != "POS" || l.ProjectName != "ESB POS" || len(l.Statuses) != 4 || len(l.NumberFields) != 4 {
		t.Fatalf("lookup = %+v, %v", l, err)
	}
	h.svc.Lookup(ctx, "POS", false)
	if h.jira.calls != 1 {
		t.Errorf("jira calls = %d, want 1 (cached 10 minutes)", h.jira.calls)
	}
	h.now = h.now.Add(11 * time.Minute)
	h.svc.Lookup(ctx, "POS", false)
	if h.jira.calls != 2 {
		t.Errorf("jira calls = %d, want 2 after the cache expired", h.jira.calls)
	}
	if _, err := h.svc.Lookup(ctx, "1POS", false); !errors.Is(err, ErrInvalidProjectKey) {
		t.Errorf("bad key err = %v", err)
	}
	if _, err := h.svc.Lookup(ctx, "NOPE", false); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("unknown project err = %v", err)
	}
	h.jira.err = errors.New("timeout")
	if _, err := h.svc.Lookup(ctx, "HR", false); !errors.Is(err, ErrJiraUnavailable) {
		t.Errorf("jira down err = %v", err)
	}
}

func TestLookup_FreshBypassesTheCache(t *testing.T) {
	h := newHarness()
	h.svc.Lookup(ctx, "POS", false)
	h.jira.statuses["POS"] = append(h.jira.statuses["POS"], "Testing")
	l, err := h.svc.Lookup(ctx, "POS", true)
	if err != nil || h.jira.calls != 2 || len(l.Statuses) != 5 {
		t.Fatalf("fresh lookup = %v, %v (calls %d), want Jira re-read with the new status", l.Statuses, err, h.jira.calls)
	}
	if again, _ := h.svc.Lookup(ctx, "POS", false); len(again.Statuses) != 5 || h.jira.calls != 2 {
		t.Errorf("the fresh result must refresh the cache: %v (calls %d)", again.Statuses, h.jira.calls)
	}
}

func TestCreate_RechecksJiraOnceWhenTheCacheLacksAStatus(t *testing.T) {
	h := newHarness(seedRules()...)
	h.svc.Lookup(ctx, "POS", false) // cached without "Testing"
	h.jira.statuses["POS"] = append(h.jira.statuses["POS"], "Testing")
	in := validPOS()
	in.TrackedStatuses = append(in.TrackedStatuses, "Testing")
	if _, err := h.svc.Create(ctx, in, "admin@esb.co.id"); err != nil {
		t.Fatalf("err = %v, want the freshly added Jira status accepted", err)
	}
	if h.jira.calls != 2 {
		t.Errorf("jira calls = %d, want 2 (cache + one fresh re-check)", h.jira.calls)
	}
	in.TrackedStatuses = append(in.TrackedStatuses, "Nope")
	var v *ValidationError
	if _, err := h.svc.Update(ctx, "POS", in, "admin@esb.co.id"); !errors.As(err, &v) {
		t.Fatalf("err = %v, want a validation error", err)
	}
	if h.jira.calls != 3 {
		t.Errorf("jira calls = %d, want 3 (one re-check, then reject)", h.jira.calls)
	}
}

func TestCreate_RejectsInputThatDoesNotMatchJira(t *testing.T) {
	cases := map[string]struct {
		mutate func(*RuleInput)
		field  string
	}{
		"bad key":            {func(in *RuleInput) { in.ProjectKey = "pos-1" }, "projectKey"},
		"no tracked":         {func(in *RuleInput) { in.TrackedStatuses, in.ReviewStatus = nil, nil }, "trackedStatuses"},
		"unknown status":     {func(in *RuleInput) { in.TrackedStatuses = append(in.TrackedStatuses, "Testing") }, "trackedStatuses"},
		"done is tracked":    {func(in *RuleInput) { in.DoneStatus = "In Progress" }, "doneStatus"},
		"no done":            {func(in *RuleInput) { in.DoneStatus = " " }, "doneStatus"},
		"review not tracked": {func(in *RuleInput) { r := "To Do"; in.ReviewStatus = &r }, "reviewStatus"},
		"no fields":          {func(in *RuleInput) { in.StoryPointFields = nil }, "storyPointFields"},
		"not a number field": {func(in *RuleInput) { in.StoryPointFields = append(in.StoryPointFields, "customfield_999") }, "storyPointFields"},
		"duplicate field":    {func(in *RuleInput) { in.StoryPointFields = append(in.StoryPointFields, "customfield_10016") }, "storyPointFields"},
		"write not read":     {func(in *RuleInput) { in.StoryPointWriteField = "customfield_11053" }, "storyPointWriteField"},
		"no subtask write":   {func(in *RuleInput) { in.StoryPointWriteFieldSubtask = "" }, "storyPointWriteFieldSubtask"},
	}
	for name, c := range cases {
		h := newHarness(seedRules()...)
		in := validPOS()
		c.mutate(&in)
		_, err := h.svc.Create(ctx, in, "admin@esb.co.id")
		var v *ValidationError
		if !errors.As(err, &v) || v.Fields[c.field] == "" {
			t.Errorf("%s: err = %v, want a %s error (AC3.7)", name, err, c.field)
		}
		if _, stored := h.store.rules["POS"]; stored {
			t.Errorf("%s: an invalid rule was stored", name)
		}
	}
}

func TestCreate_StoresJiraSpellingReloadsAndRefusesDuplicates(t *testing.T) {
	h := newHarness(seedRules()...)
	got, err := h.svc.Create(ctx, validPOS(), "admin@esb.co.id")
	if err != nil {
		t.Fatal(err)
	}
	stored := h.store.rules["POS"]
	if !reflect.DeepEqual(stored.TrackedStatuses, []string{"In Progress", "Code Review"}) || stored.DoneStatus != "Done" ||
		stored.ReviewStatus != "Code Review" || stored.ProjectName != "ESB POS" || !stored.IsActive || stored.EditedBy != "admin@esb.co.id" {
		t.Fatalf("stored = %+v", stored)
	}
	if keys := h.lastAppliedKeys(); !reflect.DeepEqual(keys, []string{"CORE", "OMS", "POS"}) {
		t.Errorf("registry = %v, want POS added right after the save (AC3.4)", keys)
	}
	if got.ProjectKey != "POS" || got.ReviewStatus == nil || *got.ReviewStatus != "Code Review" ||
		!reflect.DeepEqual(got.StoryPointFields, []FieldRef{{ID: "customfield_10016", Name: "Story point estimate"}, {ID: "customfield_200", Name: "Effort"}}) {
		t.Errorf("view = %+v", got)
	}
	if _, err := h.svc.Create(ctx, validPOS(), "admin@esb.co.id"); !errors.Is(err, ErrRuleExists) {
		t.Errorf("duplicate err = %v, want ErrRuleExists (AC3.6)", err)
	}
	in := validPOS()
	in.ProjectKey = "NOPE"
	if _, err := h.svc.Create(ctx, in, "admin@esb.co.id"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("unknown project err = %v (AC3.5)", err)
	}
}

func TestUpdate_KeepsTheActiveFlagAndTheKey(t *testing.T) {
	rules := seedRules()
	rules[1].IsActive = false // OMS
	h := newHarness(rules...)
	in := RuleInput{ProjectKey: "IGNORED", TrackedStatuses: []string{"Code Process"}, DoneStatus: "Code Review",
		StoryPointFields: []string{"customfield_10016"}, StoryPointWriteField: "customfield_10016", StoryPointWriteFieldSubtask: "customfield_10016"}
	got, err := h.svc.Update(ctx, "oms", in, "admin@esb.co.id")
	if err != nil || got.ProjectKey != "OMS" || got.IsActive || got.EditedBy != "admin@esb.co.id" || got.ReviewStatus != nil {
		t.Fatalf("update = %+v, %v", got, err)
	}
	if _, stored := h.store.rules["IGNORED"]; stored {
		t.Error("the body's projectKey must be ignored on PUT")
	}
	if _, err := h.svc.Update(ctx, "POS", in, "admin@esb.co.id"); !errors.Is(err, ErrRuleNotFound) {
		t.Errorf("unknown rule err = %v", err)
	}
}

func TestSetActive_NeverLeavesZeroActiveRules(t *testing.T) {
	rules := seedRules()
	rules[1].IsActive = false // OMS
	h := newHarness(rules...)
	if _, err := h.svc.SetActive(ctx, "CORE", false, "admin@esb.co.id"); !errors.Is(err, ErrLastActiveRule) {
		t.Fatalf("err = %v, want ErrLastActiveRule (AC4.3)", err)
	}
	if got, err := h.svc.SetActive(ctx, "oms", true, "admin@esb.co.id"); err != nil || !got.IsActive {
		t.Fatalf("activate OMS = %+v, %v", got, err)
	}
	if _, err := h.svc.SetActive(ctx, "CORE", false, "admin@esb.co.id"); err != nil {
		t.Fatal(err)
	}
	if keys := h.lastAppliedKeys(); !reflect.DeepEqual(keys, []string{"OMS"}) {
		t.Errorf("registry = %v, want only OMS (AC4.2)", keys)
	}
	if _, err := h.svc.SetActive(ctx, "POS", true, "admin@esb.co.id"); !errors.Is(err, ErrRuleNotFound) {
		t.Errorf("unknown rule err = %v", err)
	}
}

func TestList_ShowsFieldNamesAndFallsBackToIDs(t *testing.T) {
	h := newHarness(seedRules()...)
	list, err := h.svc.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	want := []FieldRef{{ID: "customfield_10125", Name: "Story Point Estimate"}, {ID: "customfield_11053", Name: "Story Points"}}
	if !reflect.DeepEqual(list[0].StoryPointFields, want) || list[0].ReviewStatus == nil || list[1].ReviewStatus != nil {
		t.Errorf("CORE/OMS views = %+v", list)
	}
	down := newHarness(seedRules()...)
	down.jira.err = errors.New("jira down")
	list, err = down.svc.List(ctx)
	if err != nil || list[1].StoryPointFields[0] != (FieldRef{ID: "customfield_10016", Name: "customfield_10016"}) {
		t.Errorf("with jira down = %+v, %v; want the ID as name", list, err)
	}
}

func TestRecalculate_OneRunAtATime(t *testing.T) {
	h := newHarness(seedRules()...)
	recalc := &fakeRecalc{started: make(chan struct{}, 1), release: make(chan struct{})}
	h.svc.d.Recalc = recalc
	if err := h.svc.Recalculate(); err != nil {
		t.Fatal(err)
	}
	<-recalc.started
	if err := h.svc.Recalculate(); !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("second call err = %v, want ErrAlreadyRunning", err)
	}
	close(recalc.release)
}

func TestRecalculate_StopsOnShutdown(t *testing.T) {
	h := newHarness(seedRules()...)
	lifetime, shutdown := context.WithCancel(ctx)
	recalc := &fakeRecalc{started: make(chan struct{}, 1), release: make(chan struct{}), stopped: make(chan error, 1)}
	h.svc.d.Recalc, h.svc.d.Lifetime = recalc, lifetime
	if err := h.svc.Recalculate(); err != nil {
		t.Fatal(err)
	}
	<-recalc.started
	shutdown()
	select {
	case err := <-recalc.stopped:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("run ctx err = %v, want canceled by shutdown", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the recalculation ignored the shutdown")
	}
}

func TestBoardStatuses_CachedPerProjectForAnHour(t *testing.T) {
	h := newHarness()
	got, err := h.svc.BoardStatuses(ctx, 26)
	if err != nil || !reflect.DeepEqual(got, newFakeJira().statuses["CORE"]) {
		t.Fatalf("statuses = %v, %v", got, err)
	}
	h.jira.err = errors.New("jira down")
	if again, err := h.svc.BoardStatuses(ctx, 26); err != nil || len(again) != 5 {
		t.Errorf("cached = %v, %v", again, err)
	}
	h.now = h.now.Add(61 * time.Minute)
	if _, err := h.svc.BoardStatuses(ctx, 26); !errors.Is(err, ErrJiraUnavailable) {
		t.Errorf("expired + jira down err = %v", err)
	}
	for _, id := range []int64{99, 7} { // board without project / unknown board
		if _, err := h.svc.BoardStatuses(ctx, id); !errors.Is(err, ErrBoardNotFound) {
			t.Errorf("board %d err = %v", id, err)
		}
	}
}
