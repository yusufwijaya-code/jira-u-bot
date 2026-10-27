package chat_interactive_middleware

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeTokenVerifier struct {
	err error
}

func (f *fakeTokenVerifier) Verify(ctx context.Context, token, audience string) error {
	return f.err
}

func TestVerifyChatRequest_ValidTokenCallsNext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/jira-notif/chat-events", nil)
	c.Request.Header.Set("Authorization", "Bearer valid-token")

	handler := VerifyChatRequest(&fakeTokenVerifier{err: nil}, "https://labcup.tech/jira-notif/chat-events")
	handler(c)

	if c.IsAborted() {
		t.Error("request was aborted, want it to pass through to the next handler")
	}
}

func TestVerifyChatRequest_MissingAuthorizationHeaderReturns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/jira-notif/chat-events", nil)

	handler := VerifyChatRequest(&fakeTokenVerifier{err: nil}, "https://labcup.tech/jira-notif/chat-events")
	handler(c)

	if w.Code != 401 {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if !c.IsAborted() {
		t.Error("expected request to be aborted when Authorization header is missing")
	}
}

func TestVerifyChatRequest_InvalidSignatureReturns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/jira-notif/chat-events", nil)
	c.Request.Header.Set("Authorization", "Bearer bad-signature-token")

	handler := VerifyChatRequest(&fakeTokenVerifier{err: errors.New("idtoken: invalid signature")}, "https://labcup.tech/jira-notif/chat-events")
	handler(c)

	if w.Code != 401 {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestVerifyChatRequest_AudienceMismatchReturns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/jira-notif/chat-events", nil)
	c.Request.Header.Set("Authorization", "Bearer token-for-a-different-endpoint")

	handler := VerifyChatRequest(&fakeTokenVerifier{err: errors.New("idtoken: audience mismatch")}, "https://labcup.tech/jira-notif/chat-events")
	handler(c)

	if w.Code != 401 {
		t.Errorf("status = %d, want 401", w.Code)
	}
}
