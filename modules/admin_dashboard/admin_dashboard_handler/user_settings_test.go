package admin_dashboard_handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/modules/admin_dashboard/admin_dashboard_dto"
	"nudge/modules/admin_dashboard/admin_dashboard_service"
)

func TestAdminTarget_BadIDIs400(t *testing.T) {
	w, c := newAdminTestContext("GET", "/x", "", gin.Params{{Key: "userID", Value: "abc"}})
	if _, ok := AdminTarget(&fakeAdminDashboardService{})(c); ok || w.Code != http.StatusBadRequest {
		t.Errorf("ok=%v status=%d, want false/400", ok, w.Code)
	}
}

func TestAdminTarget_MissingUserIs404(t *testing.T) {
	svc := &fakeAdminDashboardService{getUserErr: admin_dashboard_service.ErrUserNotFound}
	w, c := newAdminTestContext("GET", "/x", "", gin.Params{{Key: "userID", Value: "9"}})
	if _, ok := AdminTarget(svc)(c); ok || w.Code != http.StatusNotFound {
		t.Errorf("ok=%v status=%d, want false/404", ok, w.Code)
	}
}

func TestAdminTarget_ResolvesUser(t *testing.T) {
	svc := &fakeAdminDashboardService{user: admin_dashboard_dto.UserRow{UserID: 9, Email: "t@esb.co.id"}}
	_, c := newAdminTestContext("GET", "/x", "", gin.Params{{Key: "userID", Value: "9"}})
	target, ok := AdminTarget(svc)(c)
	if !ok || target.UserID != 9 || target.Email != "t@esb.co.id" {
		t.Errorf("target=%+v ok=%v, want user 9 t@esb.co.id", target, ok)
	}
	if svc.lastGetUserID != 9 {
		t.Errorf("GetUser called with %d, want 9", svc.lastGetUserID)
	}
}

func TestGetUser_Handler(t *testing.T) {
	svc := &fakeAdminDashboardService{user: admin_dashboard_dto.UserRow{UserID: 9, Email: "t@esb.co.id"}}
	w, c := newAdminTestContext("GET", "/x", "", gin.Params{{Key: "userID", Value: "9"}})
	NewAdminDashboardHandler(svc).GetUser(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"email":"t@esb.co.id"`) {
		t.Errorf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetUser_HandlerNotFound(t *testing.T) {
	svc := &fakeAdminDashboardService{getUserErr: admin_dashboard_service.ErrUserNotFound}
	w, c := newAdminTestContext("GET", "/x", "", gin.Params{{Key: "userID", Value: "9"}})
	NewAdminDashboardHandler(svc).GetUser(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}
