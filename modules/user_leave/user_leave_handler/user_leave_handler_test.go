package user_leave_handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/user_leave/user_leave_dto"
	"nudge/modules/user_leave/user_leave_service"
)

type fakeLeaveService struct {
	addErr, removeErr error
	gotEmail          string
	gotFrom, gotTo    string
}

func (f *fakeLeaveService) ForEmail(context.Context, string) business_hours_dto.HolidayCalendar {
	return business_hours_dto.HolidayCalendar{}
}
func (f *fakeLeaveService) List(_ context.Context, email string) ([]user_leave_dto.Leave, error) {
	f.gotEmail = email
	return nil, nil
}
func (f *fakeLeaveService) Add(_ context.Context, email, from, to, _ string) (user_leave_dto.AddResult, error) {
	f.gotEmail, f.gotFrom, f.gotTo = email, from, to
	return user_leave_dto.AddResult{Added: []string{from}, Skipped: []string{}}, f.addErr
}
func (f *fakeLeaveService) Remove(_ context.Context, email, _ string) (bool, error) {
	f.gotEmail = email
	return true, f.removeErr
}

// perform runs one request through the real routes as the session user
// dev@esb.co.id.
func perform(svc user_leave_service.UserLeaveService, method, path, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("", func(c *gin.Context) {
		c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "dev@esb.co.id"})
	})
	h := NewUserLeaveHandler(svc)
	api.GET("/leaves", h.List)
	api.POST("/leaves", h.Add)
	api.DELETE("/leaves/:date", h.Remove)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestAdd_MapsErrorsToStatus(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, http.StatusOK},
		{user_leave_service.ErrInvalidRange, http.StatusBadRequest},
		{user_leave_service.ErrNoWorkingDay, http.StatusUnprocessableEntity},
		{errors.New("db down"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		svc := &fakeLeaveService{addErr: c.err}
		w := perform(svc, http.MethodPost, "/leaves?email=victim@esb.co.id", `{"from":"2026-11-23","to":"2026-11-24","note":"x"}`)
		if w.Code != c.want {
			t.Errorf("err %v → %d, want %d", c.err, w.Code, c.want)
		}
		if svc.gotEmail != "dev@esb.co.id" || svc.gotFrom != "2026-11-23" || svc.gotTo != "2026-11-24" {
			t.Errorf("service got %q %q..%q, want the session user's range", svc.gotEmail, svc.gotFrom, svc.gotTo)
		}
	}
}

func TestAdd_MissingFromIs400(t *testing.T) {
	w := perform(&fakeLeaveService{}, http.MethodPost, "/leaves", `{"to":"2026-11-24"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", w.Code)
	}
}

func TestRemove_MapsErrorsToStatus(t *testing.T) {
	if w := perform(&fakeLeaveService{removeErr: user_leave_service.ErrNotFound}, http.MethodDelete, "/leaves/2026-11-18", ""); w.Code != http.StatusNotFound {
		t.Errorf("not found → %d, want 404", w.Code)
	}
	w := perform(&fakeLeaveService{}, http.MethodDelete, "/leaves/2026-11-18", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"recalculating":true`) {
		t.Errorf("ok → %d %s", w.Code, w.Body.String())
	}
}

func TestList_EmptyIsArray(t *testing.T) {
	w := perform(&fakeLeaveService{}, http.MethodGet, "/leaves", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"leaves":[]`) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}
