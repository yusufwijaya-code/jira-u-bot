package chat_app_handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/modules/chat_app/chat_app_dto"
)

type fakeVerifier struct{ err error }

func (f fakeVerifier) Verify(context.Context, string) error { return f.err }

type fakeSvc struct {
	panics bool
	got    chat_app_dto.Event
}

func (f *fakeSvc) Handle(_ context.Context, ev chat_app_dto.Event) chat_app_dto.Response {
	if f.panics {
		panic("boom")
	}
	f.got = ev
	return chat_app_dto.Response{Text: "ok " + ev.Type}
}

func do(h *Handler, auth, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/chat/events", h.Events)
	req := httptest.NewRequest(http.MethodPost, "/api/chat/events", strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const event = `{"type":"MESSAGE","space":{"name":"spaces/A","type":"DM"},"user":{"name":"users/1","email":"dev@esb.co.id"}}`

func TestEvents(t *testing.T) {
	svc := &fakeSvc{}
	if w := do(New(fakeVerifier{}, svc), "", event); w.Code != http.StatusUnauthorized {
		t.Errorf("no token: %d", w.Code)
	}
	if w := do(New(fakeVerifier{err: errors.New("bad")}, svc), "Bearer x", event); w.Code != http.StatusUnauthorized || w.Body.String() != `{"error":"unauthorized"}` {
		t.Errorf("bad token: %d %s", w.Code, w.Body.String())
	}
	if svc.got.Type != "" {
		t.Errorf("Handle ran for a rejected token: %+v", svc.got)
	}
	if w := do(New(fakeVerifier{}, svc), "Bearer x", "{not json"); w.Code != http.StatusBadRequest {
		t.Errorf("bad body: %d", w.Code)
	}
	w := do(New(fakeVerifier{}, svc), "Bearer x", event)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"text":"ok MESSAGE"`) || svc.got.User.Email != "dev@esb.co.id" {
		t.Errorf("ok: %d %s", w.Code, w.Body.String())
	}
	w = do(New(fakeVerifier{}, &fakeSvc{panics: true}), "Bearer x", event)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Terjadi kesalahan") {
		t.Errorf("panic: %d %s", w.Code, w.Body.String())
	}
}

func TestEvents_OversizedBodyIsRejected(t *testing.T) {
	svc := &fakeSvc{}
	body := `{"type":"MESSAGE","pad":"` + strings.Repeat("x", maxEventBytes) + `"}`
	if w := do(New(fakeVerifier{}, svc), "Bearer x", body); w.Code != http.StatusBadRequest || svc.got.Type != "" {
		t.Errorf("oversized body: %d, handled %+v", w.Code, svc.got)
	}
}

func TestRejectLog_SamplesLines(t *testing.T) {
	var r rejectLog
	for i := 0; i < 5; i++ {
		r.log(errors.New("bad"))
	}
	if r.suppressed != 4 {
		t.Errorf("suppressed = %d, want 4", r.suppressed)
	}
}
