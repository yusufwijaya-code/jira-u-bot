package tracking_rule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
)

type fakeAPI struct {
	err       error
	gotKey    string
	gotEditor string
	gotActive *bool
	gotInput  RuleInput
	gotFresh  bool
}

func (f *fakeAPI) List(context.Context) ([]RuleView, error) {
	return []RuleView{{ProjectKey: "CORE"}}, f.err
}
func (f *fakeAPI) Lookup(_ context.Context, key string, fresh bool) (Lookup, error) {
	f.gotKey, f.gotFresh = key, fresh
	return Lookup{ProjectKey: "POS"}, f.err
}
func (f *fakeAPI) Create(_ context.Context, in RuleInput, editor string) (RuleView, error) {
	f.gotInput, f.gotEditor = in, editor
	return RuleView{ProjectKey: "POS"}, f.err
}
func (f *fakeAPI) Update(_ context.Context, key string, in RuleInput, editor string) (RuleView, error) {
	f.gotKey, f.gotInput, f.gotEditor = key, in, editor
	return RuleView{ProjectKey: key}, f.err
}
func (f *fakeAPI) SetActive(_ context.Context, key string, active bool, editor string) (RuleView, error) {
	f.gotKey, f.gotActive, f.gotEditor = key, &active, editor
	return RuleView{ProjectKey: key, IsActive: active}, f.err
}
func (f *fakeAPI) Recalculate() error { return f.err }
func (f *fakeAPI) BoardStatuses(context.Context, int64) ([]string, error) {
	return []string{"To Do"}, f.err
}

func serve(api *fakeAPI, method, path, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth := func(c *gin.Context) {
		c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "admin@esb.co.id"})
	}
	h := NewHandler(api)
	AdminRoutes(r.Group("/api/dashboard/admin", auth), h)
	UserRoutes(r.Group("/api/dashboard", auth), h)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestRoutes_HappyPaths(t *testing.T) {
	cases := []struct {
		method, path, body string
		code               int
		frag               string
	}{
		{http.MethodGet, "/api/dashboard/admin/tracking-rules", "", http.StatusOK, `"rules":[{"projectKey":"CORE"`},
		{http.MethodGet, "/api/dashboard/admin/tracking-rules/jira/pos", "", http.StatusOK, `"projectKey":"POS"`},
		{http.MethodPost, "/api/dashboard/admin/tracking-rules", `{"projectKey":"POS","trackedStatuses":["In Progress"]}`, http.StatusCreated, `"projectKey":"POS"`},
		{http.MethodPut, "/api/dashboard/admin/tracking-rules/CORE", `{"doneStatus":"Done"}`, http.StatusOK, `"projectKey":"CORE"`},
		{http.MethodPut, "/api/dashboard/admin/tracking-rules/OMS/active", `{"active":false}`, http.StatusOK, `"isActive":false`},
		{http.MethodPost, "/api/dashboard/admin/tracking-rules/recalculate", ``, http.StatusAccepted, `"started":true`},
		{http.MethodGet, "/api/dashboard/boards/26/statuses", "", http.StatusOK, `"statuses":["To Do"]`},
	}
	for _, c := range cases {
		api := &fakeAPI{}
		w := serve(api, c.method, c.path, c.body)
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.frag) {
			t.Errorf("%s %s = %d %s, want %d with %s", c.method, c.path, w.Code, w.Body, c.code, c.frag)
		}
	}
	api := &fakeAPI{}
	serve(api, http.MethodPost, "/api/dashboard/admin/tracking-rules", `{"projectKey":"POS","trackedStatuses":["In Progress"]}`)
	if api.gotEditor != "admin@esb.co.id" || api.gotInput.ProjectKey != "POS" || len(api.gotInput.TrackedStatuses) != 1 {
		t.Errorf("create got editor=%q input=%+v", api.gotEditor, api.gotInput)
	}
	serve(api, http.MethodGet, "/api/dashboard/admin/tracking-rules/jira/pos", "")
	if api.gotFresh {
		t.Error("lookup without ?fresh=1 must use the cache")
	}
	serve(api, http.MethodGet, "/api/dashboard/admin/tracking-rules/jira/pos?fresh=1", "")
	if !api.gotFresh || api.gotKey != "pos" {
		t.Errorf("lookup ?fresh=1 got fresh=%v key=%q", api.gotFresh, api.gotKey)
	}
	serve(api, http.MethodPut, "/api/dashboard/admin/tracking-rules/OMS/active", `{"active":false}`)
	if api.gotKey != "OMS" || api.gotActive == nil || *api.gotActive {
		t.Errorf("set active got key=%q active=%v", api.gotKey, api.gotActive)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		code int
		frag string
	}{
		{&ValidationError{Fields: map[string]string{"doneStatus": "wajib"}}, http.StatusBadRequest, `"fields":{"doneStatus":"wajib"}`},
		{ErrInvalidProjectKey, http.StatusBadRequest, `"error":"invalid_project_key"`},
		{ErrProjectNotFound, http.StatusNotFound, `Project POS tidak ditemukan di Jira`},
		{ErrRuleNotFound, http.StatusNotFound, `"error":"rule_not_found"`},
		{ErrRuleExists, http.StatusConflict, `"error":"rule_exists"`},
		{ErrLastActiveRule, http.StatusUnprocessableEntity, `"error":"last_active_rule"`},
		{ErrAlreadyRunning, http.StatusConflict, `"error":"already_running"`},
		{ErrBoardNotFound, http.StatusNotFound, `"error":"board_not_found"`},
		{ErrJiraUnavailable, http.StatusBadGateway, `"error":"jira_unavailable"`},
		{context.DeadlineExceeded, http.StatusInternalServerError, `"error":"internal_error"`},
	}
	for _, c := range cases {
		w := serve(&fakeAPI{err: c.err}, http.MethodPost, "/api/dashboard/admin/tracking-rules", `{"projectKey":"pos"}`)
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.frag) {
			t.Errorf("%v: %d %s, want %d with %s", c.err, w.Code, w.Body, c.code, c.frag)
		}
	}
}

func TestBadRequests(t *testing.T) {
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/dashboard/admin/tracking-rules", `{`},
		{http.MethodPut, "/api/dashboard/admin/tracking-rules/CORE/active", `{}`},
		{http.MethodGet, "/api/dashboard/boards/abc/statuses", ``},
	} {
		if w := serve(&fakeAPI{}, c.method, c.path, c.body); w.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d, want 400", c.method, c.path, w.Code)
		}
	}
}

// The user route shares /boards/:boardId with the existing avatar route —
// a different wildcard name would make gin panic at startup.
func TestUserRoutes_ShareTheBoardsWildcard(t *testing.T) {
	r := gin.New()
	g := r.Group("/api/dashboard")
	g.GET("/boards/:boardId/avatar", func(*gin.Context) {})
	UserRoutes(g, NewHandler(&fakeAPI{}))
}
