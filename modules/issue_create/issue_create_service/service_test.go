package issue_create_service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_client_service"
	"nudge/modules/user_dashboard/user_dashboard_dto"
)

type fakeJira struct {
	types    []jira_client_dto.CreateMetaIssueType
	fields   map[string][]jira_client_dto.CreateMetaField
	created  []map[string]any
	failOn   string
	brief    jira_client_dto.IssueBrief
	metaHits int
	search   []jira_client_dto.IssueBrief
	lastJQL  string
}

func (f *fakeJira) SearchBriefsAs(_ context.Context, _ *jira_client_dto.Credential, jql string, _ int) ([]jira_client_dto.IssueBrief, error) {
	f.lastJQL = jql
	return f.search, nil
}

func (f *fakeJira) IssueTypesAs(_ context.Context, _ *jira_client_dto.Credential, _ string) ([]jira_client_dto.CreateMetaIssueType, error) {
	f.metaHits++
	return f.types, nil
}
func (f *fakeJira) IssueTypeFieldsAs(_ context.Context, _ *jira_client_dto.Credential, _, id string) ([]jira_client_dto.CreateMetaField, error) {
	return f.fields[id], nil
}
func (f *fakeJira) CreateIssueAs(_ context.Context, _ *jira_client_dto.Credential, fields map[string]any) (string, error) {
	if f.failOn != "" && fields["summary"] == f.failOn {
		return "", &jira_client_service.RejectedError{Fields: map[string]string{"summary": "bad"}}
	}
	f.created = append(f.created, fields)
	return "CORE-" + string(rune('0'+len(f.created))), nil
}
func (f *fakeJira) AssignableUsersAs(context.Context, *jira_client_dto.Credential, string, string) ([]jira_client_dto.AssignableUser, error) {
	return nil, nil
}
func (f *fakeJira) IssueBriefAs(_ context.Context, _ *jira_client_dto.Credential, key string) (jira_client_dto.IssueBrief, error) {
	if f.brief.Key != key {
		return jira_client_dto.IssueBrief{}, jira_client_service.ErrIssueNotFound
	}
	return f.brief, nil
}
func (f *fakeJira) MyselfAs(context.Context, *jira_client_dto.Credential) (string, error) {
	return "me", nil
}

type fakeCreds struct{ none bool }

func (f fakeCreds) ResolveCredential(context.Context, string) (*jira_client_dto.Credential, error) {
	if f.none {
		return nil, nil
	}
	return &jira_client_dto.Credential{Email: "dev@esb.co.id", APIToken: "t"}, nil
}

type fakeBoards struct{}

func (fakeBoards) ListBoards(context.Context, string) ([]user_dashboard_dto.BoardSummary, error) {
	return []user_dashboard_dto.BoardSummary{{BoardID: 26, ProjectKey: "CORE"}, {BoardID: 28, ProjectKey: "OMS"}}, nil
}

type fakeAI struct {
	calls int
	reply string
	err   error
}

func (f *fakeAI) Configured() bool { return true }
func (f *fakeAI) GenerateJSON(_ context.Context, _, user string, _ map[string]any, out any) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	if !strings.Contains(user, "<<<BREAKDOWN") {
		panic("user text must be wrapped in delimiters")
	}
	return jsonUnmarshal(f.reply, out)
}

func omsJira() *fakeJira {
	return &fakeJira{
		types: []jira_client_dto.CreateMetaIssueType{{ID: "10139", Name: "Story"}, {ID: "10143", Name: "SUB Story", Subtask: true}, {ID: "9", Name: "Weird"}},
		fields: map[string][]jira_client_dto.CreateMetaField{
			"10139": {
				{FieldID: "summary", Name: "Summary", Required: true, Schema: jira_client_dto.FieldSchema{Type: "string"}},
				{FieldID: "customfield_10016", Name: "Story point estimate", Schema: jira_client_dto.FieldSchema{Type: "number"}},
				{FieldID: "customfield_12001", Name: "Merge Request", Required: true, Schema: jira_client_dto.FieldSchema{Type: "string"}},
				{FieldID: "customfield_12002", Name: "Cloud Services", Required: true, Schema: jira_client_dto.FieldSchema{Type: "option"}, AllowedValues: []jira_client_dto.AllowedValue{{ID: "1", Value: "OMS SERVICE"}}},
				{FieldID: "parent", Name: "Parent", Schema: jira_client_dto.FieldSchema{Type: "issuelink"}},
			},
			"10143": {
				{FieldID: "parent", Name: "Parent", Required: true, Schema: jira_client_dto.FieldSchema{Type: "issuelink"}},
				{FieldID: "customfield_10016", Name: "Story point estimate", Schema: jira_client_dto.FieldSchema{Type: "number"}},
			},
			"9": {{FieldID: "customfield_9", Name: "Thing", Required: true, Schema: jira_client_dto.FieldSchema{Type: "sd-customerrequesttype"}}},
		},
	}
}

func newSvc(j *fakeJira, ai *fakeAI) *Service {
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	return New(Deps{Jira: j, Credentials: fakeCreds{}, Boards: fakeBoards{}, AI: ai, Now: func() time.Time { return now }})
}

func TestMeta_MapsDynamicFieldsAndCaches(t *testing.T) {
	j := omsJira()
	s := newSvc(j, &fakeAI{})
	m, err := s.Meta(context.Background(), "dev@esb.co.id", 28)
	if err != nil {
		t.Fatal(err)
	}
	if m.ProjectKey != "OMS" || m.SubtaskTypeID != "10143" {
		t.Fatalf("meta = %+v", m)
	}
	story := m.Types[0]
	if story.StoryPointField != "customfield_10016" || !story.Supported || !story.ParentAllowed || len(story.Fields) != 2 {
		t.Fatalf("story = %+v (want SP field, supported, parent allowed, 2 dynamic fields)", story)
	}
	if story.Fields[1].Kind != "select" || story.Fields[1].Options[0].Label != "OMS SERVICE" {
		t.Fatalf("select field = %+v", story.Fields[1])
	}
	if m.Types[2].Supported {
		t.Fatal("type with an unknown required field must be unsupported")
	}
	s.Meta(context.Background(), "dev@esb.co.id", 28)
	if j.metaHits != 1 {
		t.Fatalf("createmeta fetched %d times, want 1 (cached)", j.metaHits)
	}
}

func TestMeta_CacheInvalidatedWhenTrackingRuleChanges(t *testing.T) {
	t.Cleanup(func() { jira_client_dto.SetRules(jira_client_dto.BuiltinRules()) })
	j := coreJira()
	s := newSvc(j, &fakeAI{})
	if _, err := s.Meta(context.Background(), "dev@esb.co.id", 26); err != nil {
		t.Fatal(err)
	}
	rules := jira_client_dto.BuiltinRules()
	for i := range rules {
		if rules[i].ProjectKey == "CORE" {
			rules[i].StoryPointWriteField = "customfield_11053"
		}
	}
	jira_client_dto.SetRules(rules)
	m, err := s.Meta(context.Background(), "dev@esb.co.id", 26)
	if err != nil {
		t.Fatal(err)
	}
	if j.metaHits != 2 {
		t.Fatalf("createmeta fetched %d times, want 2 (rule change invalidates the cache)", j.metaHits)
	}
	if m.Types[0].StoryPointField != "customfield_11053" {
		t.Fatalf("story SP field = %q, want the new write field", m.Types[0].StoryPointField)
	}
	s.Meta(context.Background(), "dev@esb.co.id", 26)
	if j.metaHits != 2 {
		t.Fatalf("createmeta fetched %d times, want 2 (unchanged rule stays cached)", j.metaHits)
	}
}

func TestMeta_RejectsUnregisteredBoardAndMissingToken(t *testing.T) {
	s := newSvc(omsJira(), &fakeAI{})
	if _, err := s.Meta(context.Background(), "dev@esb.co.id", 99); !errors.Is(err, ErrBoardNotAllowed) {
		t.Fatalf("err = %v", err)
	}
	s2 := New(Deps{Jira: omsJira(), Credentials: fakeCreds{none: true}, Boards: fakeBoards{}, AI: &fakeAI{}})
	if _, err := s2.Meta(context.Background(), "dev@esb.co.id", 28); !errors.Is(err, ErrTokenRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateOne_EncodesFieldsAndDefaultsAssigneeToMe(t *testing.T) {
	j := omsJira()
	s := newSvc(j, &fakeAI{})
	sp := 3.0
	key, err := s.CreateOne(context.Background(), "dev@esb.co.id", CreateRequest{
		BoardID: 28, TypeID: "10139", Summary: "Hello", Description: "line 1\nline 2", StoryPoints: &sp,
		Fields: map[string]any{"customfield_12001": "https://mr", "customfield_12002": "1"},
	})
	if err != nil || key == "" {
		t.Fatalf("key = %q, err = %v", key, err)
	}
	f := j.created[0]
	if f["customfield_10016"] != 3.0 || f["customfield_12001"] != "https://mr" {
		t.Fatalf("fields = %v", f)
	}
	if opt, ok := f["customfield_12002"].(map[string]any); !ok || opt["id"] != "1" {
		t.Fatalf("select must be sent as {id}, got %v", f["customfield_12002"])
	}
	if a := f["assignee"].(map[string]any); a["accountId"] != "me" {
		t.Fatalf("assignee = %v", f["assignee"])
	}
	if f["project"].(map[string]any)["key"] != "OMS" || f["issuetype"].(map[string]any)["id"] != "10139" {
		t.Fatalf("project/issuetype = %v / %v", f["project"], f["issuetype"])
	}
	if doc := f["description"].(map[string]any); len(doc["content"].([]any)) != 2 {
		t.Fatalf("description ADF = %v", doc)
	}
}

func TestCreateOne_ValidatesRequiredDynamicFields(t *testing.T) {
	s := newSvc(omsJira(), &fakeAI{})
	_, err := s.CreateOne(context.Background(), "dev@esb.co.id", CreateRequest{BoardID: 28, TypeID: "10139", Summary: "x"})
	var v *ValidationError
	if !errors.As(err, &v) || !strings.Contains(v.Message, "Merge Request") {
		t.Fatalf("err = %v", err)
	}
}

func TestNormalize_SanitizesCachesAndLimits(t *testing.T) {
	ai := &fakeAI{reply: `{"rows":[{"title":"  [Exec Plan] **Do it**  ","storyPoints":4.5,"details":"Repo: semua"},{"title":"","storyPoints":1,"details":""},{"title":"Bad SP","storyPoints":500,"details":""}],"warnings":[]}`}
	s := newSvc(omsJira(), ai)
	n, err := s.Normalize(context.Background(), "dev@esb.co.id", 26, "| # | Task |")
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Rows) != 2 || n.Rows[0].Title != "[Exec Plan] Do it" || *n.Rows[0].StoryPoints != 4.5 || n.Rows[1].StoryPoints != nil {
		t.Fatalf("rows = %+v", n.Rows)
	}
	if len(n.Warnings) == 0 {
		t.Fatal("want a warning for the invalid SP")
	}
	again, _ := s.Normalize(context.Background(), "dev@esb.co.id", 26, "| # | Task |")
	if !again.Cached || ai.calls != 1 {
		t.Fatalf("second call cached=%v calls=%d, want cached and 1 call", again.Cached, ai.calls)
	}
	s.Normalize(context.Background(), "dev@esb.co.id", 26, "b")
	s.Normalize(context.Background(), "dev@esb.co.id", 26, "c")
	_, err = s.Normalize(context.Background(), "dev@esb.co.id", 26, "d")
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.Scope != "user" {
		t.Fatalf("4th uncached normalize in a minute: err = %v, want user rate limit", err)
	}
}

func TestNormalize_RejectsTooLongText(t *testing.T) {
	s := newSvc(omsJira(), &fakeAI{reply: `{"rows":[],"warnings":[]}`})
	_, err := s.Normalize(context.Background(), "dev@esb.co.id", 26, strings.Repeat("x", 30001))
	var v *ValidationError
	if !errors.As(err, &v) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateSubtasks_SequentialWithPartialFailure(t *testing.T) {
	j := omsJira()
	j.brief.Key = "OMS-5"
	j.brief.Fields.Project.Key = "OMS"
	j.failOn = "B"
	s := newSvc(j, &fakeAI{})
	res, err := s.CreateSubtasks(context.Background(), "dev@esb.co.id", 28, "OMS-5", []BulkRow{{ClientID: "1", Title: "A"}, {ClientID: "2", Title: "B"}, {ClientID: "3", Title: "C"}})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Key == "" || res[1].Error == "" || res[2].Key == "" {
		t.Fatalf("results = %+v", res)
	}
	if j.created[0]["parent"].(map[string]any)["key"] != "OMS-5" || j.created[0]["issuetype"].(map[string]any)["id"] != "10143" {
		t.Fatalf("subtask fields = %v", j.created[0])
	}
	if j.created[1]["summary"] != "C" {
		t.Fatal("rows must be created in order")
	}
}

func TestParent_RejectsOtherProjectAndSubtasks(t *testing.T) {
	j := omsJira()
	j.brief.Key = "CORE-1"
	j.brief.Fields.Project.Key = "CORE"
	s := newSvc(j, &fakeAI{})
	if _, err := s.Parent(context.Background(), "dev@esb.co.id", 28, "CORE-1"); !errors.Is(err, ErrParentInvalid) {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.Parent(context.Background(), "dev@esb.co.id", 28, "OMS-404"); !errors.Is(err, ErrParentNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestParent_RejectsEpicWithTypeName(t *testing.T) {
	j := omsJira()
	j.brief.Key = "OMS-9"
	j.brief.Fields.Project.Key = "OMS"
	j.brief.Fields.IssueType.Name = "Epic"
	j.brief.Fields.IssueType.HierarchyLevel = 1
	_, err := newSvc(j, &fakeAI{}).Parent(context.Background(), "dev@esb.co.id", 28, "oms-9")
	var pi *ParentInvalidError
	if !errors.Is(err, ErrParentInvalid) || !errors.As(err, &pi) || pi.Message != "Epic tidak bisa punya subtask" {
		t.Fatalf("err = %v", err)
	}
	j.brief.Fields.Project.Key = "CORE"
	if _, err := newSvc(j, &fakeAI{}).Parent(context.Background(), "dev@esb.co.id", 28, "OMS-9"); !errors.As(err, &pi) || !strings.Contains(pi.Message, "project lain") {
		t.Fatalf("err = %v", err)
	}
}

func TestParent_ReturnsTypeAndStatus(t *testing.T) {
	j := omsJira()
	j.brief.Key = "OMS-5"
	j.brief.Fields.Project.Key = "OMS"
	j.brief.Fields.IssueType.Name = "Story"
	j.brief.Fields.IssueType.IconURL = "https://i/s.svg"
	j.brief.Fields.Status.Name = "To Do"
	j.brief.Fields.Status.StatusCategory.Key = "new"
	p, err := newSvc(j, &fakeAI{}).Parent(context.Background(), "dev@esb.co.id", 28, "OMS-5")
	if err != nil || p.Type != "Story" || p.TypeIconURL == "" || p.Status != "To Do" || p.StatusCategory != "new" {
		t.Fatalf("p = %+v, err = %v", p, err)
	}
}

func brief(key string, level int, subtask bool, subs int) jira_client_dto.IssueBrief {
	var b jira_client_dto.IssueBrief
	b.Key = key
	b.Fields.Summary = key + " summary"
	b.Fields.IssueType.HierarchyLevel = level
	b.Fields.IssueType.Subtask = subtask
	for i := 0; i < subs; i++ {
		b.Fields.Subtasks = append(b.Fields.Subtasks, struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
			} `json:"fields"`
		}{Key: "x"})
	}
	return b
}

func TestParents_FiltersEpicsAndSubtasks(t *testing.T) {
	j := omsJira()
	j.search = []jira_client_dto.IssueBrief{brief("OMS-1", 0, false, 2), brief("OMS-2", 1, false, 0), brief("OMS-3", -1, true, 0), brief("OMS-4", 0, false, 0)}
	got, err := newSvc(j, &fakeAI{}).Parents(context.Background(), "dev@esb.co.id", 28, "")
	if err != nil || len(got) != 2 || got[0].Key != "OMS-1" || got[0].SubtaskCount != 2 || got[1].Key != "OMS-4" {
		t.Fatalf("got = %+v, err = %v", got, err)
	}
	if !strings.Contains(j.lastJQL, `project = "OMS" AND sprint in openSprints() AND issuetype not in subTaskIssueTypes()`) || !strings.HasSuffix(j.lastJQL, "ORDER BY updated DESC") {
		t.Fatalf("jql = %s", j.lastJQL)
	}
}

func TestParentsJQL_KeyOrSummary(t *testing.T) {
	if q := parentsJQL("CORE", " core-12 "); !strings.Contains(q, `AND key = "CORE-12"`) || strings.Contains(q, "summary") {
		t.Fatalf("key jql = %s", q)
	}
	if q := parentsJQL("CORE", `say "hi" (now)`); !strings.Contains(q, `AND summary ~ "say hi now*"`) {
		t.Fatalf("summary jql = %s", q)
	}
	if q := parentsJQL("CORE", `***`); strings.Contains(q, "summary") {
		t.Fatalf("operator-only q must not filter: %s", q)
	}
	if got := jqlQuote(`a"b\c`); got != `a\"b\\c` {
		t.Fatalf("jqlQuote = %s", got)
	}
}

func jsonUnmarshal(s string, out any) error { return json.Unmarshal([]byte(s), out) }

func TestMeta_NoIssueTypesYieldsEmptyNotNil(t *testing.T) {
	j := omsJira()
	j.types = nil
	m, err := newSvc(j, &fakeAI{}).Meta(context.Background(), "dev@esb.co.id", 28)
	if err != nil {
		t.Fatal(err)
	}
	if m.Types == nil || len(m.Types) != 0 {
		t.Fatalf("Types = %#v, want non-nil empty", m.Types)
	}
}

func TestNormalize_DoesNotCacheEmptyResult(t *testing.T) {
	ai := &fakeAI{reply: `{"rows":[],"warnings":[]}`}
	s := newSvc(omsJira(), ai)
	s.Normalize(context.Background(), "dev@esb.co.id", 26, "x")
	again, _ := s.Normalize(context.Background(), "dev@esb.co.id", 26, "x")
	if again.Cached || ai.calls != 2 {
		t.Fatalf("cached=%v calls=%d, want uncached and 2 calls", again.Cached, ai.calls)
	}
}

func coreJira() *fakeJira {
	number := jira_client_dto.FieldSchema{Type: "number"}
	return &fakeJira{
		types: []jira_client_dto.CreateMetaIssueType{{ID: "10001", Name: "Story"}, {ID: "10003", Name: "Subtask", Subtask: true}, {ID: "10004", Name: "Bug"}},
		fields: map[string][]jira_client_dto.CreateMetaField{
			"10001": {
				{FieldID: "customfield_10125", Name: "Story Point Estimate", Required: true, Schema: number},
				{FieldID: "customfield_11053", Name: "Story Points", Schema: number},
			},
			"10003": {
				{FieldID: "parent", Name: "Parent", Required: true, Schema: jira_client_dto.FieldSchema{Type: "issuelink"}},
				{FieldID: "customfield_10125", Name: "Story Point Estimate", Schema: number},
				{FieldID: "customfield_11053", Name: "Story Points", Schema: number},
			},
			"10004": {{FieldID: "customfield_11053", Name: "Story Points", Schema: number}},
		},
	}
}

func TestMeta_CoreWritesEstimateOnCardsAndStoryPointsOnSubtasks(t *testing.T) {
	m, err := newSvc(coreJira(), &fakeAI{}).Meta(context.Background(), "dev@esb.co.id", 26)
	if err != nil {
		t.Fatal(err)
	}
	story, sub, bug := m.Types[0], m.Types[1], m.Types[2]
	if story.StoryPointField != "customfield_10125" || len(story.Fields) != 0 {
		t.Fatalf("story = %+v, want SP on customfield_10125 and no dynamic field (SP fields are handled)", story)
	}
	if sub.StoryPointField != "customfield_11053" {
		t.Fatalf("subtask = %+v, want SP on customfield_11053 (AC5.3)", sub)
	}
	if bug.StoryPointField != "customfield_11053" {
		t.Fatalf("bug = %+v, want Story Points as the fallback: customfield_10125 is not on its createmeta", bug)
	}
}

func TestStoryPointFieldFor_PrefersTheWriteFieldThenReadOrder(t *testing.T) {
	read := []string{"customfield_10125", "customfield_11053"}
	cases := []struct {
		name, preferred string
		onType          map[string]bool
		want            string
	}{
		{"preferred present", "customfield_10125", map[string]bool{"customfield_10125": true, "customfield_11053": true}, "customfield_10125"},
		{"fallback to Story Points", "customfield_10125", map[string]bool{"customfield_11053": true}, "customfield_11053"},
		{"subtask prefers Story Points", "customfield_11053", map[string]bool{"customfield_10125": true, "customfield_11053": true}, "customfield_11053"},
		{"no SP field on the type", "customfield_10125", map[string]bool{"summary": true}, ""},
	}
	for _, c := range cases {
		if got := storyPointFieldFor(c.preferred, read, c.onType); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCreateSubtasks_CoreWritesTheSubtaskField(t *testing.T) {
	j := coreJira()
	j.brief.Key = "CORE-5"
	j.brief.Fields.Project.Key = "CORE"
	pts := 2.0
	res, err := newSvc(j, &fakeAI{}).CreateSubtasks(context.Background(), "dev@esb.co.id", 26, "CORE-5", []BulkRow{{ClientID: "1", Title: "A", StoryPoints: &pts}})
	if err != nil || res[0].Key == "" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if j.created[0]["customfield_11053"] != 2.0 || j.created[0]["customfield_10125"] != nil {
		t.Fatalf("subtask fields = %v, want SP on customfield_11053 only", j.created[0])
	}
}
