package admin_dashboard_handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/admin_dashboard/admin_dashboard_dto"
	"jira-u-bot/modules/admin_dashboard/admin_dashboard_service"
	"jira-u-bot/modules/auth/auth_dto"
	"jira-u-bot/modules/auth/auth_middleware"
)

type fakeAdminDashboardService struct {
	users             []admin_dashboard_dto.UserRow
	setActiveErr      error
	lastSetActiveArgs [3]int64 // acting, target, active(as 0/1)
	deleteUserErr     error
	lastDeletedUserID int64
	updatedAccountID  string
	deletedCredID     int64
}

func (f *fakeAdminDashboardService) ListUsers(ctx context.Context) ([]admin_dashboard_dto.UserRow, error) {
	return f.users, nil
}
func (f *fakeAdminDashboardService) SetActive(ctx context.Context, actingUserID, targetUserID int64, active bool) error {
	activeInt := int64(0)
	if active {
		activeInt = 1
	}
	f.lastSetActiveArgs = [3]int64{actingUserID, targetUserID, activeInt}
	return f.setActiveErr
}
func (f *fakeAdminDashboardService) UpdateJiraAccountID(ctx context.Context, targetUserID int64, newAccountID string) error {
	f.updatedAccountID = newAccountID
	return nil
}
func (f *fakeAdminDashboardService) DeleteUser(ctx context.Context, actingUserID, targetUserID int64) error {
	f.lastDeletedUserID = targetUserID
	return f.deleteUserErr
}
func (f *fakeAdminDashboardService) DeleteUserCredential(ctx context.Context, targetUserID int64) error {
	f.deletedCredID = targetUserID
	return nil
}

func newAdminTestContext(method, path, body string, params gin.Params) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	if body != "" {
		c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
	} else {
		c.Request = httptest.NewRequest(method, path, nil)
	}
	c.Params = params
	c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "yusuf.wijaya@esb.co.id"})
	return w, c
}

func TestUserList_RendersUsers(t *testing.T) {
	svc := &fakeAdminDashboardService{users: []admin_dashboard_dto.UserRow{
		{UserID: 2, Email: "dev@esb.co.id", FlagActive: true},
	}}
	handler := NewAdminDashboardHandler(svc)

	w, c := newAdminTestContext("GET", "/dashboard/admin", "", nil)
	handler.UserList(c)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "dev@esb.co.id") {
		t.Errorf("status=%d body missing dev@esb.co.id", w.Code)
	}
}

func TestToggleActive_CallsServiceWithActingAndTargetID(t *testing.T) {
	svc := &fakeAdminDashboardService{}
	handler := NewAdminDashboardHandler(svc)

	w, c := newAdminTestContext("POST", "/dashboard/admin/users/2/active", `{"active":false}`, gin.Params{{Key: "userID", Value: "2"}})
	handler.ToggleActive(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if svc.lastSetActiveArgs != [3]int64{1, 2, 0} {
		t.Errorf("SetActive args = %v, want [actingUserID=1 targetUserID=2 active=0]", svc.lastSetActiveArgs)
	}
}

func TestToggleActive_ReturnsBadRequestOnSelfLockout(t *testing.T) {
	svc := &fakeAdminDashboardService{setActiveErr: admin_dashboard_service.ErrCannotModifySelf}
	handler := NewAdminDashboardHandler(svc)

	w, c := newAdminTestContext("POST", "/dashboard/admin/users/1/active", `{"active":false}`, gin.Params{{Key: "userID", Value: "1"}})
	handler.ToggleActive(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (spec self-lockout guard)", w.Code, http.StatusBadRequest)
	}
}

func TestEditJiraAccountID_UpdatesService(t *testing.T) {
	svc := &fakeAdminDashboardService{}
	handler := NewAdminDashboardHandler(svc)

	w, c := newAdminTestContext("POST", "/dashboard/admin/users/2/jira-account-id", `{"jiraAccountId":"acc-new"}`, gin.Params{{Key: "userID", Value: "2"}})
	handler.EditJiraAccountID(c)

	if w.Code != http.StatusOK || svc.updatedAccountID != "acc-new" {
		t.Errorf("status=%d updatedAccountID=%q, want 200 and acc-new", w.Code, svc.updatedAccountID)
	}
}

func TestDeleteUser_DelegatesToService(t *testing.T) {
	svc := &fakeAdminDashboardService{}
	handler := NewAdminDashboardHandler(svc)

	w, c := newAdminTestContext("DELETE", "/dashboard/admin/users/2", "", gin.Params{{Key: "userID", Value: "2"}})
	handler.DeleteUser(c)

	if w.Code != http.StatusOK || svc.lastDeletedUserID != 2 {
		t.Errorf("status=%d deletedUserID=%d, want 200 and 2", w.Code, svc.lastDeletedUserID)
	}
}

func TestDeleteUserCredential_DelegatesToService(t *testing.T) {
	svc := &fakeAdminDashboardService{}
	handler := NewAdminDashboardHandler(svc)

	w, c := newAdminTestContext("DELETE", "/dashboard/admin/users/2/credential", "", gin.Params{{Key: "userID", Value: "2"}})
	handler.DeleteUserCredential(c)

	if w.Code != http.StatusOK || svc.deletedCredID != 2 {
		t.Errorf("status=%d deletedCredID=%d, want 200 and 2", w.Code, svc.deletedCredID)
	}
}
