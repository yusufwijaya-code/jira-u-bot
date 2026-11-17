package issue_create_handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	svc "nudge/modules/issue_create/issue_create_service"
)

type fakeSvc struct{ err error }

func (f fakeSvc) Status(context.Context, string) (svc.Status, error) {
	return svc.Status{TokenReady: true}, f.err
}
func (f fakeSvc) Meta(context.Context, string, int64) (svc.Meta, error) {
	return svc.Meta{ProjectKey: "CORE"}, f.err
}
func (f fakeSvc) Users(context.Context, string, int64, string) ([]svc.User, error) { return nil, f.err }
func (f fakeSvc) Parent(context.Context, string, int64, string) (svc.Parent, error) {
	return svc.Parent{}, f.err
}
func (f fakeSvc) Parents(_ context.Context, _ string, _ int64, q string) ([]svc.Candidate, error) {
	return []svc.Candidate{{Key: "CORE-1", Summary: q, Type: "Story", SubtaskCount: 2}}, f.err
}
func (f fakeSvc) CreateOne(context.Context, string, svc.CreateRequest) (string, error) {
	return "CORE-1", f.err
}
func (f fakeSvc) Normalize(context.Context, string, int64, string) (svc.Normalized, error) {
	return svc.Normalized{}, f.err
}
func (f fakeSvc) CreateSubtasks(context.Context, string, int64, string, []svc.BulkRow) ([]svc.BulkResult, error) {
	return nil, f.err
}

func serve(f fakeSvc, method, path, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/dashboard", func(c *gin.Context) {
		c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "dev@esb.co.id"})
	})
	Routes(g, New(f, "https://esb-id.atlassian.net"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		code int
		frag string
	}{
		{svc.ErrTokenRequired, http.StatusPreconditionFailed, "jira_token_required"},
		{svc.ErrBoardNotAllowed, http.StatusBadRequest, "board"},
		{&svc.ValidationError{Message: "Judul wajib"}, http.StatusBadRequest, "Judul wajib"},
		{&svc.ValidationError{Message: "Due date: tanggal harus YYYY-MM-DD", Field: "duedate"}, http.StatusBadRequest, `"fields":{"duedate":"Due date: tanggal harus YYYY-MM-DD"}`},
		{svc.ErrParentNotFound, http.StatusNotFound, "parent"},
		{svc.ErrParentInvalid, http.StatusUnprocessableEntity, "parent"},
		{&svc.ParentInvalidError{Message: "Epic tidak bisa punya subtask"}, http.StatusUnprocessableEntity, `"error":"Epic tidak bisa punya subtask"`},
		{&svc.RateLimitError{Scope: "daily", RetryAfterSeconds: 60}, http.StatusTooManyRequests, `"scope":"daily"`},
		{svc.ErrAIUnavailable, http.StatusServiceUnavailable, "ai"},
	}
	for _, tc := range cases {
		w := serve(fakeSvc{err: tc.err}, http.MethodPost, "/api/dashboard/create/normalize", `{"boardId":26,"text":"x"}`)
		if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.frag) {
			t.Errorf("%v → %d %s, want %d containing %q", tc.err, w.Code, w.Body.String(), tc.code, tc.frag)
		}
	}
}

func TestCreateIssueReturnsURL(t *testing.T) {
	w := serve(fakeSvc{}, http.MethodPost, "/api/dashboard/create/issue", `{"boardId":26,"typeId":"1","summary":"x"}`)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), "https://esb-id.atlassian.net/browse/CORE-1") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestMetaNeedsBoardID(t *testing.T) {
	if w := serve(fakeSvc{}, http.MethodGet, "/api/dashboard/create/meta", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", w.Code)
	}
}

func TestParentsListsCandidates(t *testing.T) {
	w := serve(fakeSvc{}, http.MethodGet, "/api/dashboard/create/parents?boardId=26&q=login", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"parents":[{"key":"CORE-1","summary":"login"`) || !strings.Contains(w.Body.String(), `"subtaskCount":2`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if w := serve(fakeSvc{}, http.MethodGet, "/api/dashboard/create/parents", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("missing boardId → %d", w.Code)
	}
}
