package chat_app_activation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/base/helpers/curl_helper"
	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/chat_app/chat_app_dto"
	"nudge/modules/user_dashboard/user_dashboard_handler"
)

type fakeStates struct {
	states map[string]chat_app_dto.State
	set    []string
	sender map[string]*string
}

func (f *fakeStates) FindByEmail(_ context.Context, email string) (chat_app_dto.State, bool, error) {
	s, ok := f.states[email]
	return s, ok, nil
}
func (f *fakeStates) SetSpace(_ context.Context, email, space, user string) error {
	f.set = append(f.set, email+"|"+space)
	s := f.states[email]
	s.SpaceID = space
	f.states[email] = s
	return nil
}
func (f *fakeStates) SetSender(_ context.Context, email string, sender *string) error {
	if f.sender == nil {
		f.sender = map[string]*string{}
	}
	f.sender[email] = sender
	s := f.states[email]
	s.Sender = sender
	f.states[email] = s
	return nil
}

type fakeOAuth struct {
	email string
	err   error
}

func (f fakeOAuth) AuthCodeURL(state, hint string) string {
	return "https://accounts.example/auth?state=" + state + "&login_hint=" + hint
}
func (f fakeOAuth) Exchange(context.Context, string) (string, string, error) {
	return "user-token", f.email, f.err
}

type fakeNotify struct{ sent chan string }

func (f fakeNotify) SendPlainTextDM(_ context.Context, email, text string) error {
	f.sent <- email + "|" + text
	return nil
}

const (
	me = "dev@esb.co.id"
	op = "bot@esb.co.id"
)

func newHandler(states *fakeStates, oauth OAuth, setup SpaceSetup, notify Notifier) *Handler {
	return New(Deps{
		States: states, OAuth: oauth, SetupDM: setup, Notify: notify,
		AppEnabled: true, OneClick: true, OperatorEmail: op, OperatorName: "Yusuf Wijaya",
		AppURL: "https://nudge.labcup.tech", CookiePath: "",
	})
}

func engine(h *Handler, email string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 7, Email: email, FlagActive: true})
	})
	r.GET("/status", h.Status(user_dashboard_handler.SessionTarget))
	r.PUT("/sender", h.SetSender(user_dashboard_handler.SessionTarget, nil))
	r.GET("/activate", h.Activate)
	r.GET("/callback", h.Callback)
	return r
}

func call(r *gin.Engine, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestStatus(t *testing.T) {
	states := &fakeStates{states: map[string]chat_app_dto.State{me: {Email: me}}}
	w := call(engine(newHandler(states, fakeOAuth{}, nil, nil), me), http.MethodGet, "/status", "")
	var got StatusResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if !got.Enabled || got.Activated || got.EffectiveSender != "operator" || got.OperatorAccount || got.OperatorName != "Yusuf Wijaya" || !got.OneClick {
		t.Errorf("status = %+v", got)
	}

	w = call(engine(newHandler(states, nil, nil, nil), op), http.MethodGet, "/status", "")
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if !got.OperatorAccount || got.EffectiveSender != "nudge" || got.OneClick {
		t.Errorf("operator status = %+v (oneClick must be false without OAuth)", got)
	}
}

func TestSetSender(t *testing.T) {
	notify := fakeNotify{sent: make(chan string, 2)}
	states := &fakeStates{states: map[string]chat_app_dto.State{me: {Email: me}, op: {Email: op}}}
	h := newHandler(states, fakeOAuth{}, nil, notify)

	if w := call(engine(h, me), http.MethodPut, "/sender", `{"sender":"x"}`); w.Code != http.StatusBadRequest {
		t.Errorf("bad value: %d", w.Code)
	}
	if w := call(engine(h, op), http.MethodPut, "/sender", `{"sender":"operator"}`); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("operator account → operator: %d", w.Code)
	}
	w := call(engine(h, me), http.MethodPut, "/sender", `{"sender":"nudge"}`)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "chat_app_not_activated") {
		t.Errorf("nudge before activation: %d %s", w.Code, w.Body.String())
	}

	w = call(engine(h, me), http.MethodPut, "/sender", `{"sender":"operator"}`)
	if w.Code != http.StatusOK || states.sender[me] == nil || *states.sender[me] != "operator" {
		t.Fatalf("operator: %d %v", w.Code, states.sender)
	}
	select {
	case msg := <-notify.sent:
		if !strings.HasPrefix(msg, me+"|✅") {
			t.Errorf("confirmation = %q", msg)
		}
	case <-time.After(time.Second):
		t.Error("no confirmation DM")
	}

	disabled := New(Deps{States: states, AppEnabled: false, OperatorEmail: op})
	if w := call(engine(disabled, me), http.MethodPut, "/sender", `{"sender":"nudge"}`); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("nudge with app disabled: %d", w.Code)
	}
}

func TestActivateAndCallback(t *testing.T) {
	states := &fakeStates{states: map[string]chat_app_dto.State{me: {Email: me}}}
	setup := func(_ context.Context, token string) (string, error) {
		if token != "user-token" {
			t.Errorf("token = %q", token)
		}
		return "spaces/NEW", nil
	}
	r := engine(newHandler(states, fakeOAuth{email: me}, setup, nil), me)

	w := call(r, http.MethodGet, "/activate?then=sender", "")
	if w.Code != http.StatusFound || !strings.Contains(w.Header().Get("Location"), "login_hint=dev@esb.co.id") {
		t.Fatalf("activate: %d %s", w.Code, w.Header().Get("Location"))
	}
	cookie := w.Result().Cookies()[0]
	state := strings.SplitN(cookie.Value, "|", 2)[0]

	w = call(r, http.MethodGet, "/callback?state="+state+"&code=abc", "", cookie)
	if loc := w.Header().Get("Location"); loc != "https://nudge.labcup.tech/dashboard/settings?chatApp=activated" {
		t.Fatalf("callback redirect = %q", loc)
	}
	if len(states.set) != 1 || states.sender[me] == nil || *states.sender[me] != "nudge" {
		t.Errorf("set=%v sender=%v", states.set, states.sender)
	}
}

func TestCallbackErrors(t *testing.T) {
	cases := []struct {
		name  string
		oauth fakeOAuth
		setup SpaceSetup
		query string
		state bool
		want  string
	}{
		{"bad state", fakeOAuth{email: me}, nil, "state=other&code=c", true, "failed"},
		{"denied", fakeOAuth{email: me}, nil, "error=access_denied", true, "cancelled"},
		{"other google error", fakeOAuth{email: me}, nil, "error=org_internal", true, "blocked"},
		{"exchange error", fakeOAuth{err: errors.New("x")}, nil, "code=c", true, "failed"},
		{"other account", fakeOAuth{email: "else@esb.co.id"}, nil, "code=c", true, "mismatch"},
		{"not in visibility", fakeOAuth{email: me}, func(context.Context, string) (string, error) {
			return "", &curl_helper.HTTPError{StatusCode: 403}
		}, "code=c", true, "not_allowed"},
		{"setup error", fakeOAuth{email: me}, func(context.Context, string) (string, error) {
			return "", errors.New("down")
		}, "code=c", true, "failed"},
	}
	for _, c := range cases {
		states := &fakeStates{states: map[string]chat_app_dto.State{me: {Email: me}}}
		r := engine(newHandler(states, c.oauth, c.setup, nil), me)
		cookie := &http.Cookie{Name: stateCookieName, Value: "S1|"}
		q := c.query
		if c.state && !strings.Contains(q, "state=") {
			q += "&state=S1"
		}
		w := call(r, http.MethodGet, "/callback?"+q, "", cookie)
		if want := "https://nudge.labcup.tech/dashboard?chatApp=" + c.want; w.Header().Get("Location") != want {
			t.Errorf("%s: Location = %q, want %q", c.name, w.Header().Get("Location"), want)
		}
	}
}
