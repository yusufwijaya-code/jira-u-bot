package chat_log_handler

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

	"nudge/modules/chat_delivery"
	"nudge/modules/chat_log/chat_log_dto"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/user_setting/user_setting_dto"
)

type fakeLister struct {
	got chat_log_dto.Filter
	err error
}

func (f *fakeLister) List(_ context.Context, filter chat_log_dto.Filter) (chat_log_dto.Page, error) {
	f.got = filter
	return chat_log_dto.Page{Items: []chat_log_dto.Entry{{ID: 7, Body: "halo"}}, NextBeforeID: 7}, f.err
}

func (f *fakeLister) Conversations(context.Context) ([]chat_log_dto.Conversation, error) {
	return []chat_log_dto.Conversation{{Email: "dev@esb.co.id", Total: 3, LastBody: "halo"}}, f.err
}

func call(t *testing.T, l *fakeLister, query string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/chat-logs", New(l, nil, nil).List)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/chat-logs"+query, nil))
	return w
}

func TestList_PassesFiltersWithInclusiveEndDay(t *testing.T) {
	l := &fakeLister{}
	w := call(t, l, "?email=dev@esb.co.id&from=2026-11-01&to=2026-11-15&beforeID=20&afterID=3&limit=10&kind=reminder,command")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body)
	}
	wantFrom := time.Date(2026, 11, 1, 0, 0, 0, 0, user_setting_dto.Jakarta)
	wantTo := time.Date(2026, 11, 16, 0, 0, 0, 0, user_setting_dto.Jakarta)
	if l.got.Email != "dev@esb.co.id" || !l.got.From.Equal(wantFrom) || !l.got.To.Equal(wantTo) || l.got.BeforeID != 20 || l.got.AfterID != 3 || l.got.Limit != 10 || len(l.got.Kinds) != 2 {
		t.Errorf("filter = %+v", l.got)
	}
	var page chat_log_dto.Page
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.NextBeforeID != 7 {
		t.Errorf("body = %s", w.Body)
	}
}

func TestList_RejectsBadParams(t *testing.T) {
	for _, q := range []string{"?from=15-11-2026", "?to=x", "?beforeID=abc", "?afterID=x", "?limit=-1", "?kind=drop%20table"} {
		if w := call(t, &fakeLister{}, q); w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d", q, w.Code)
		}
	}
}

func TestList_StoreErrorIs500WithoutDetails(t *testing.T) {
	w := call(t, &fakeLister{err: errors.New("db password wrong")}, "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
	if body := w.Body.String(); body == "" || strings.Contains(body, "password") {
		t.Errorf("body leaks details: %s", body)
	}
}

func TestConversations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/c", New(&fakeLister{}, nil, nil).Conversations)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"conversations":[{"email":"dev@esb.co.id"`) {
		t.Fatalf("status = %d %s", w.Code, w.Body)
	}
}

type fakeSender struct {
	err    error
	sentTo string
	text   string
}

func (f *fakeSender) SendAdminMessage(_ context.Context, email, text string) error {
	f.sentTo, f.text = email, text
	return f.err
}

type fakeUsers map[string]registration_dto.User

func (f fakeUsers) FindUserByEmail(_ context.Context, email string) (*registration_dto.User, bool, error) {
	u, ok := f[email]
	if !ok {
		return nil, false, nil
	}
	return &u, true, nil
}

func postSend(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/send", h.Send)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

var users = fakeUsers{
	"dev@esb.co.id": {Email: "dev@esb.co.id", FlagActive: true},
	"off@esb.co.id": {Email: "off@esb.co.id", FlagActive: false},
}

func TestSend_DeliversToARegisteredUser(t *testing.T) {
	s := &fakeSender{}
	w := postSend(t, New(&fakeLister{}, s, users), `{"email":" dev@esb.co.id ","text":"  halo tim  "}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"sent"`) {
		t.Fatalf("status = %d %s", w.Code, w.Body)
	}
	if s.sentTo != "dev@esb.co.id" || s.text != "halo tim" {
		t.Errorf("sent %q to %q", s.text, s.sentTo)
	}
}

func TestSend_SkippedWhenNoSenderReachesTheUser(t *testing.T) {
	w := postSend(t, New(&fakeLister{}, &fakeSender{err: chat_delivery.ErrRecipientNotActivated}, users), `{"email":"dev@esb.co.id","text":"x"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"skipped"`) {
		t.Fatalf("status = %d %s", w.Code, w.Body)
	}
}

func TestSend_Rejections(t *testing.T) {
	cases := map[string]struct {
		h    *Handler
		body string
		code int
	}{
		"bad json":      {New(&fakeLister{}, &fakeSender{}, users), `{`, http.StatusBadRequest},
		"empty text":    {New(&fakeLister{}, &fakeSender{}, users), `{"email":"dev@esb.co.id","text":"  "}`, http.StatusBadRequest},
		"too long":      {New(&fakeLister{}, &fakeSender{}, users), `{"email":"dev@esb.co.id","text":"` + strings.Repeat("x", MaxMessageRunes+1) + `"}`, http.StatusBadRequest},
		"unknown user":  {New(&fakeLister{}, &fakeSender{}, users), `{"email":"who@gmail.com","text":"x"}`, http.StatusNotFound},
		"inactive user": {New(&fakeLister{}, &fakeSender{}, users), `{"email":"off@esb.co.id","text":"x"}`, http.StatusUnprocessableEntity},
		"send fails":    {New(&fakeLister{}, &fakeSender{err: errors.New("oauth down")}, users), `{"email":"dev@esb.co.id","text":"x"}`, http.StatusBadGateway},
		"no sender":     {New(&fakeLister{}, nil, nil), `{"email":"dev@esb.co.id","text":"x"}`, http.StatusServiceUnavailable},
	}
	for name, tc := range cases {
		if w := postSend(t, tc.h, tc.body); w.Code != tc.code {
			t.Errorf("%s: status = %d, want %d (%s)", name, w.Code, tc.code, w.Body)
		}
	}
}
