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

	"nudge/modules/chat_log/chat_log_dto"
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

func call(t *testing.T, l *fakeLister, query string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/chat-logs", New(l).List)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/chat-logs"+query, nil))
	return w
}

func TestList_PassesFiltersWithInclusiveEndDay(t *testing.T) {
	l := &fakeLister{}
	w := call(t, l, "?email=dev@esb.co.id&from=2026-11-01&to=2026-11-15&beforeID=20&limit=10")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body)
	}
	wantFrom := time.Date(2026, 11, 1, 0, 0, 0, 0, user_setting_dto.Jakarta)
	wantTo := time.Date(2026, 11, 16, 0, 0, 0, 0, user_setting_dto.Jakarta)
	if l.got.Email != "dev@esb.co.id" || !l.got.From.Equal(wantFrom) || !l.got.To.Equal(wantTo) || l.got.BeforeID != 20 || l.got.Limit != 10 {
		t.Errorf("filter = %+v", l.got)
	}
	var page chat_log_dto.Page
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.NextBeforeID != 7 {
		t.Errorf("body = %s", w.Body)
	}
}

func TestList_RejectsBadParams(t *testing.T) {
	for _, q := range []string{"?from=15-11-2026", "?to=x", "?beforeID=abc", "?limit=-1"} {
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
