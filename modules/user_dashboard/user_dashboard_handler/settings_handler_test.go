package user_dashboard_handler

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func targetResolver(id int64, email string) TargetResolver {
	return func(c *gin.Context) (SettingsTarget, bool) {
		return SettingsTarget{UserID: id, Email: email}, true
	}
}

type recordedChange struct {
	target SettingsTarget
	change SettingsChange
}

func newTargetSettingsHandler(settings *fakeUserSettingsForHandler, cred *fakeJiraCredentialForHandler, dash *fakeUserDashboardService, changes *[]recordedChange) *SettingsHandler {
	return NewSettingsHandler(dash, cred, settings, targetResolver(42, "target@esb.co.id"),
		func(c *gin.Context, t SettingsTarget, ch SettingsChange) {
			*changes = append(*changes, recordedChange{t, ch})
		})
}

func TestSettingsHandler_UsesResolvedTargetNotSession(t *testing.T) {
	settings := &fakeUserSettingsForHandler{}
	var changes []recordedChange
	h := newTargetSettingsHandler(settings, &fakeJiraCredentialForHandler{}, &fakeUserDashboardService{}, &changes)

	w, c := newTestContextWithUser("PUT", "/x?board=28", `{"reminderTiers":["1h"],"overageTiers":[],"overageRepeatMinutes":5,"notifyEarlyFinish":true,"notifyReturnedToTodo":true,"quietHours":null}`)
	h.UpdateNotification(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if settings.savedFor != 42 || settings.savedBoard != 28 {
		t.Errorf("saved for user %d board %d, want 42/28", settings.savedFor, settings.savedBoard)
	}
	if len(changes) != 1 || changes[0].change.Kind != ChangeNotification || changes[0].change.BoardID != 28 || changes[0].target.UserID != 42 {
		t.Errorf("changes = %+v, want one notification change for user 42 board 28", changes)
	}
}

func TestSettingsHandler_ResolverFalseStopsRequest(t *testing.T) {
	settings := &fakeUserSettingsForHandler{}
	h := NewSettingsHandler(&fakeUserDashboardService{}, &fakeJiraCredentialForHandler{}, settings,
		func(c *gin.Context) (SettingsTarget, bool) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user tidak ditemukan"})
			return SettingsTarget{}, false
		},
		nil)

	w, c := newTestContextWithUser("POST", "/x", `{"minutes":60}`)
	h.Mute(c)

	if w.Code != http.StatusNotFound || settings.mutedTo != nil {
		t.Errorf("status=%d mutedTo=%v, want 404 and no mute", w.Code, settings.mutedTo)
	}
}

func TestSettingsHandler_NoChangeOnValidationError(t *testing.T) {
	var changes []recordedChange
	h := newTargetSettingsHandler(&fakeUserSettingsForHandler{}, &fakeJiraCredentialForHandler{}, &fakeUserDashboardService{}, &changes)

	w, c := newTestContextWithUser("POST", "/x", `{"minutes":0}`)
	h.Mute(c)

	if w.Code != http.StatusBadRequest || len(changes) != 0 {
		t.Errorf("status=%d changes=%d, want 400 and none", w.Code, len(changes))
	}
}

func TestSettingsHandler_EmitsChangeForEachWrite(t *testing.T) {
	var changes []recordedChange
	cred := &fakeJiraCredentialForHandler{}
	settings := &fakeUserSettingsForHandler{}
	h := newTargetSettingsHandler(settings, cred, &fakeUserDashboardService{}, &changes)

	_, c := newTestContextWithUser("DELETE", "/x?board=28", "")
	h.ResetNotification(c)
	_, c = newTestContextWithUser("POST", "/x", `{"until":"forever"}`)
	h.Mute(c)
	_, c = newTestContextWithUser("DELETE", "/x?board=28", "")
	h.Unmute(c)
	_, c = newTestContextWithUser("POST", "/x", `{"jiraEmail":"t@esb.co.id","apiToken":"tok"}`)
	h.SaveCredential(c)
	_, c = newTestContextWithUser("DELETE", "/x", "")
	h.DeleteCredential(c)

	want := []SettingsChangeKind{ChangeReset, ChangeMute, ChangeUnmute, ChangeCredentialUpdated, ChangeCredentialDeleted}
	if len(changes) != len(want) {
		t.Fatalf("got %d changes, want %d: %+v", len(changes), len(want), changes)
	}
	for i, k := range want {
		if changes[i].change.Kind != k {
			t.Errorf("change[%d] = %s, want %s", i, changes[i].change.Kind, k)
		}
	}
	if changes[1].change.Until == nil {
		t.Error("mute change must carry Until")
	}
	if changes[0].change.BoardID != 28 || changes[2].change.BoardID != 28 {
		t.Errorf("reset/unmute board = %d/%d, want 28", changes[0].change.BoardID, changes[2].change.BoardID)
	}
	if settings.unmutedID != 42 || settings.mutedFor != 42 {
		t.Errorf("mute/unmute user = %d/%d, want 42", settings.mutedFor, settings.unmutedID)
	}
}

func TestSettingsHandler_ListBoardsUsesTargetEmail(t *testing.T) {
	dash := &fakeUserDashboardService{}
	var changes []recordedChange
	h := newTargetSettingsHandler(&fakeUserSettingsForHandler{}, &fakeJiraCredentialForHandler{}, dash, &changes)

	w, c := newTestContextWithUser("GET", "/x", "")
	h.ListBoards(c)

	if w.Code != http.StatusOK || dash.boardsEmail != "target@esb.co.id" {
		t.Errorf("status=%d email=%q, want 200 and target@esb.co.id", w.Code, dash.boardsEmail)
	}
}
