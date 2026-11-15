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

func jsonUnmarshal(s string, out any) error { return json.Unmarshal([]byte(s), out) }
