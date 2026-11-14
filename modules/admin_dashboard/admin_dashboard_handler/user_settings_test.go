package admin_dashboard_handler

import (
	"net/http"
	"strings"
	"time"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/modules/admin_dashboard/admin_dashboard_dto"
	"nudge/modules/admin_dashboard/admin_dashboard_service"
	"nudge/modules/user_dashboard/user_dashboard_handler"
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

func TestAdminChangeListener_ForwardsToService(t *testing.T) {
	svc := &fakeAdminDashboardService{}
	_, c := newAdminTestContext("PUT", "/x", "", nil)
	until := time.Now()
	AdminChangeListener(svc)(c,
		user_dashboard_handler.SettingsTarget{UserID: 9, Email: "t@esb.co.id"},
		user_dashboard_handler.SettingsChange{Kind: user_dashboard_handler.ChangeMute, BoardID: 5, Until: &until})

	if len(svc.notified) != 1 {
		t.Fatalf("notified %d times, want 1", len(svc.notified))
	}
	n := svc.notified[0]
	if n.actingUserID != 1 || n.actingEmail != "yusuf.wijaya@esb.co.id" || n.targetUserID != 9 || n.targetEmail != "t@esb.co.id" ||
		n.change.Kind != "mute" || n.change.BoardID != 5 || n.change.Until != &until {
		t.Errorf("notify = %+v", n)
	}
}

func TestDeleteUserCredential_NotifiesTarget(t *testing.T) {
	svc := &fakeAdminDashboardService{user: admin_dashboard_dto.UserRow{UserID: 2, Email: "t@esb.co.id"}}
	w, c := newAdminTestContext("DELETE", "/x", "", gin.Params{{Key: "userID", Value: "2"}})
	NewAdminDashboardHandler(svc).DeleteUserCredential(c)

	if w.Code != http.StatusOK || len(svc.notified) != 1 || svc.notified[0].change.Kind != "credential_deleted" || svc.notified[0].targetEmail != "t@esb.co.id" {
		t.Errorf("status=%d notified=%+v", w.Code, svc.notified)
	}
}
