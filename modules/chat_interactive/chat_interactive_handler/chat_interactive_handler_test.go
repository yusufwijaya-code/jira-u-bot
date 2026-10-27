package chat_interactive_handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/chat_interactive/chat_interactive_dto"
)

type fakeChatInteractiveService struct {
	response string
}

func (f *fakeChatInteractiveService) RouteCommand(ctx context.Context, event chat_interactive_dto.ChatEvent) string {
	return f.response
}

func TestHandleEvent_DecodesBodyAndReturnsTextJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewChatInteractiveHandler(&fakeChatInteractiveService{response: "⏰ hello from the bot"})

	body := `{"user":{"email":"dev@esb.co.id"},"appCommandPayload":{"appCommandMetadata":{"appCommandId":1}}}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/jira-notif/chat-events", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.HandleEvent(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "hello from the bot") {
		t.Errorf("body = %q, want it to contain the service's response text", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"text"`) {
		t.Errorf("body = %q, want a JSON object with a \"text\" key", w.Body.String())
	}
}

func TestHandleEvent_MalformedBodyReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewChatInteractiveHandler(&fakeChatInteractiveService{response: "unused"})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/jira-notif/chat-events", strings.NewReader("not json"))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.HandleEvent(c)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
