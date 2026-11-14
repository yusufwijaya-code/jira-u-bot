package user_avatar_handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/user_avatar/user_avatar_service"
)

type fakeAvatars struct {
	err       error
	gotEmail  string
	gotUserID int64
}

func (f *fakeAvatars) ByEmail(ctx context.Context, email string) ([]byte, string, error) {
	f.gotEmail = email
	return []byte("PNG"), "image/png", f.err
}

func (f *fakeAvatars) ByUserID(ctx context.Context, userID int64) ([]byte, string, error) {
	f.gotUserID = userID
	return []byte("PNG"), "image/png", f.err
}

func newCtx(params gin.Params) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x", nil)
	c.Params = params
	c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "me@esb.co.id"})
	return w, c
}

func TestMine_ServesSessionUserPicture(t *testing.T) {
	svc := &fakeAvatars{}
	w, c := newCtx(nil)
	NewUserAvatarHandler(svc).Mine(c)

	if w.Code != http.StatusOK || w.Body.String() != "PNG" || w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("status=%d body=%q ct=%q", w.Code, w.Body.String(), w.Header().Get("Content-Type"))
	}
	if svc.gotEmail != "me@esb.co.id" {
		t.Errorf("email = %q, want the session's", svc.gotEmail)
	}
	if w.Header().Get("Cache-Control") != "private, max-age=86400" {
		t.Errorf("Cache-Control = %q", w.Header().Get("Cache-Control"))
	}
}

func TestForUser_UsesPathUser(t *testing.T) {
	svc := &fakeAvatars{}
	w, c := newCtx(gin.Params{{Key: "userID", Value: "7"}})
	NewUserAvatarHandler(svc).ForUser(c)
	if w.Code != http.StatusOK || svc.gotUserID != 7 {
		t.Errorf("status=%d userID=%d", w.Code, svc.gotUserID)
	}
}

func TestForUser_BadIDIs400(t *testing.T) {
	w, c := newCtx(gin.Params{{Key: "userID", Value: "x"}})
	NewUserAvatarHandler(&fakeAvatars{}).ForUser(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestNoAvatarIs404AndJiraErrorIs502(t *testing.T) {
	w, c := newCtx(nil)
	NewUserAvatarHandler(&fakeAvatars{err: user_avatar_service.ErrNoAvatar}).Mine(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("no avatar status=%d, want 404", w.Code)
	}
	w, c = newCtx(nil)
	NewUserAvatarHandler(&fakeAvatars{err: errors.New("jira down")}).Mine(c)
	if w.Code != http.StatusBadGateway {
		t.Errorf("jira error status=%d, want 502", w.Code)
	}
}
