package registration_handler

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/registration/registration_service"
)

type fakeRegistrationServiceForHandler struct {
	err            error
	registerCalled bool
}

func (f *fakeRegistrationServiceForHandler) Register(ctx context.Context, name, email string) error {
	f.registerCalled = true
	return f.err
}

func TestShowForm_RendersFormWithNameAndEmailFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewRegistrationHandler(&fakeRegistrationServiceForHandler{})

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

func submitForm(t *testing.T, handler RegistrationHandler, name, email string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{}
	form.Set("name", name)
	form.Set("email", email)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	handler.Submit(c)
	return w
}

func TestSubmit_ValidRegistrationRendersSuccessPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewRegistrationHandler(&fakeRegistrationServiceForHandler{})

	w := submitForm(t, handler, "Anwar", "anwar@esb.co.id")

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Anwar") {
		t.Error("success page should mention the registrant's name")
	}
}

func TestSubmit_InvalidEmailFormatReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewRegistrationHandler(&fakeRegistrationServiceForHandler{})

	w := submitForm(t, handler, "Anwar", "not-an-email")

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestSubmit_MissingFieldsReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewRegistrationHandler(&fakeRegistrationServiceForHandler{})

	w := submitForm(t, handler, "", "")

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestSubmit_JiraAccountNotFoundReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewRegistrationHandler(&fakeRegistrationServiceForHandler{err: registration_service.ErrJiraAccountNotFound})

	w := submitForm(t, handler, "Ghost", "ghost@esb.co.id")

	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestSubmit_JiraLookupUnavailableReturns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewRegistrationHandler(&fakeRegistrationServiceForHandler{err: registration_service.ErrJiraLookupUnavailable})

	w := submitForm(t, handler, "Anwar", "anwar@esb.co.id")

	if w.Code != 503 {
		t.Errorf("status = %d, want 503", w.Code)
	}
}
