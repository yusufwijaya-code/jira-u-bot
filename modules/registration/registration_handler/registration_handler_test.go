package registration_handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestShowForm_RendersFormWithNameAndEmailFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewRegistrationHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/register", nil)

	handler.ShowForm(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `name="name"`) {
		t.Error("response body missing a name input field")
	}
	if !strings.Contains(body, `name="email"`) {
		t.Error("response body missing an email input field")
	}
}
