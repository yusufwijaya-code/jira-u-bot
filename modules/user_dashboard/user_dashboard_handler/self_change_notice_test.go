package user_dashboard_handler

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/user_setting/user_setting_dto"
)

type fakeSelfDM struct {
	mu    sync.Mutex
	texts []string
	to    []string
	err   error
	sent  chan struct{}
}

func (f *fakeSelfDM) SendPlainTextDM(ctx context.Context, email, text string) error {
	f.mu.Lock()
	f.texts = append(f.texts, text)
	f.to = append(f.to, email)
	f.mu.Unlock()
	if f.sent != nil {
		f.sent <- struct{}{}
	}
	return f.err
}

type fakeBoardDescriber map[int64]string

func (f fakeBoardDescriber) DescribeBoard(ctx context.Context, boardID int64) (string, string) {
	return f[boardID], ""
}

func syncSelfListener(dm *fakeSelfDM) ChangeListener {
	return (&selfNotifier{dm: dm, boards: fakeBoardDescriber{5: "ESB Core Board"}, sendSync: true}).listen
}

func TestSelfChangeNoticeText(t *testing.T) {
	until := time.Date(2026, 11, 17, 23, 58, 0, 0, user_setting_dto.Jakarta)
	forever := user_setting_dto.MuteForeverUntil
	off, on := false, true
	def := user_setting_dto.Default()
	tiers := def
	tiers.ReminderTiers = []string{"2h", "1h", "30m"}
	multi := def
	multi.OverageTiers = []string{"15m", "90m"}
	multi.OverageRepeatMinutes = nil
	multi.NotifyStatusChange = true
	multi.StatusChangeStatuses = []string{"In Review", "Done"}
	multi.QuietHours = &user_setting_dto.QuietHours{Start: "22:00", End: "07:00"}
	inherited := def
	inherited.UseDefault = true
	statusOff := def
	statusOff.NotifyStatusChange = false

	cases := []struct {
		name  string
		board string
		ch    SettingsChange
		want  string
	}{
		{"switch off", "", SettingsChange{Kind: ChangeNotificationsSwitch, Enabled: &off}, "✅ Pengaturan kamu diperbarui: Notifikasi otomatis dimatikan"},
		{"switch on", "", SettingsChange{Kind: ChangeNotificationsSwitch, Enabled: &on}, "✅ Pengaturan kamu diperbarui: Notifikasi otomatis dinyalakan"},
		{"mute board", "ESB Core Board", SettingsChange{Kind: ChangeMute, BoardID: 5, Until: &until}, "✅ Pengaturan kamu diperbarui: Mute ESB Core Board sampai Sel, 17 Nov 23.58 WIB"},
		{"mute global", "", SettingsChange{Kind: ChangeMute, Until: &until}, "✅ Pengaturan kamu diperbarui: Mute semua notifikasi sampai Sel, 17 Nov 23.58 WIB"},
		{"mute forever", "", SettingsChange{Kind: ChangeMute, Until: &forever}, "✅ Pengaturan kamu diperbarui: Mute semua notifikasi sampai kamu nyalakan lagi"},
		{"unmute board", "ESB Core Board", SettingsChange{Kind: ChangeUnmute, BoardID: 5}, "✅ Pengaturan kamu diperbarui: Mute ESB Core Board dicabut, notifikasi board ini aktif lagi"},
		{"unmute global", "", SettingsChange{Kind: ChangeUnmute}, "✅ Pengaturan kamu diperbarui: Mute semua notifikasi dicabut"},
		{"reset", "ESB Core Board", SettingsChange{Kind: ChangeReset, BoardID: 5}, "✅ Pengaturan kamu diperbarui: ESB Core Board kembali mengikuti pengaturan default"},
		{"cred saved", "", SettingsChange{Kind: ChangeCredentialUpdated}, "✅ Pengaturan kamu diperbarui: Jira credential disimpan"},
		{"cred deleted", "", SettingsChange{Kind: ChangeCredentialDeleted}, "✅ Pengaturan kamu diperbarui: Jira credential dihapus"},
		{"tiers board", "ESB Core Board", SettingsChange{Kind: ChangeNotification, BoardID: 5, Before: &def, After: &tiers},
			"✅ Pengaturan kamu diperbarui: Reminder sebelum budget habis (ESB Core Board): T-2 jam, T-1 jam, T-30 menit"},
		{"status off default", "", SettingsChange{Kind: ChangeNotification, Before: &def, After: &statusOff},
			"✅ Pengaturan kamu diperbarui: Notifikasi perubahan status (default semua board): mati"},
		{"several fields", "ESB Core Board", SettingsChange{Kind: ChangeNotification, BoardID: 5, Before: &def, After: &multi},
			"✅ Pengaturan kamu diperbarui (ESB Core Board):\n• Reminder setelah budget habis: T+15 menit, T+1,5 jam\n• Ulang setelah tier terakhir: berhenti\n• Notifikasi perubahan status: nyala (In Review, Done)\n• Jam tenang harian: 22.00–07.00 WIB"},
		{"became custom, same values", "ESB Core Board", SettingsChange{Kind: ChangeNotification, BoardID: 5, Before: &inherited, After: &def},
			"✅ Pengaturan kamu diperbarui: ESB Core Board sekarang pakai pengaturan khusus"},
		{"no diff", "", SettingsChange{Kind: ChangeNotification, Before: &def, After: &def}, ""},
		{"sender has its own DM", "", SettingsChange{Kind: ChangeChatSender}, ""},
	}
	for _, tc := range cases {
		if got := SelfChangeNoticeText(tc.board, tc.ch); got != tc.want {
			t.Errorf("%s:\n got  %q\n want %q", tc.name, got, tc.want)
		}
	}
}

func TestSelfChangeListener_SendsOneDMToTarget(t *testing.T) {
	dm := &fakeSelfDM{}
	syncSelfListener(dm)(nil, SettingsTarget{UserID: 1, Email: "dev@esb.co.id"}, SettingsChange{Kind: ChangeUnmute, BoardID: 5})
	if len(dm.texts) != 1 || dm.to[0] != "dev@esb.co.id" || dm.texts[0] != "✅ Pengaturan kamu diperbarui: Mute ESB Core Board dicabut, notifikasi board ini aktif lagi" {
		t.Errorf("sent %v to %v", dm.texts, dm.to)
	}
}

func TestSelfChangeListener_SkipsNoOp(t *testing.T) {
	dm := &fakeSelfDM{}
	syncSelfListener(dm)(nil, SettingsTarget{UserID: 1, Email: "dev@esb.co.id"}, SettingsChange{Kind: ChangeUnmute, NoOp: true})
	if len(dm.texts) != 0 {
		t.Errorf("no-op sent %v", dm.texts)
	}
}

func TestSelfChangeListener_AsyncAndSendErrorIsOnlyLogged(t *testing.T) {
	dm := &fakeSelfDM{err: errors.New("chat down"), sent: make(chan struct{}, 1)}
	listener := SelfChangeListener(dm, nil)
	listener(nil, SettingsTarget{UserID: 1, Email: "dev@esb.co.id"}, SettingsChange{Kind: ChangeCredentialDeleted})
	select {
	case <-dm.sent:
	case <-time.After(2 * time.Second):
		t.Fatal("DM never sent")
	}
}

// The session-user routes (NewUserDashboardHandler) DM the user; a failing
// send does not change the response.
func TestUserDashboardHandler_SelfChangeWiring(t *testing.T) {
	dm := &fakeSelfDM{err: errors.New("chat down")}
	settings := &fakeUserSettingsForHandler{}
	h := NewUserDashboardHandler(&fakeUserDashboardService{}, &fakeChatInteractiveForHandler{}, &fakeJiraCredentialForHandler{}, settings, nil,
		WithSelfChangeListener(syncSelfListener(dm)))

	w, c := newTestContextWithUser("PUT", "/x", `{"enabled":false}`)
	h.UpdateNotificationsSwitch(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if len(dm.texts) != 1 || dm.to[0] != "dev@esb.co.id" || dm.texts[0] != "✅ Pengaturan kamu diperbarui: Notifikasi otomatis dimatikan" {
		t.Errorf("sent %v to %v", dm.texts, dm.to)
	}
}

func TestSettingsHandler_NoOpDetection(t *testing.T) {
	var changes []recordedChange
	listener := func(c *gin.Context, t SettingsTarget, ch SettingsChange) {
		changes = append(changes, recordedChange{t, ch})
	}
	settings := &fakeUserSettingsForHandler{notifSwitch: user_setting_dto.NotificationSwitch{Enabled: true, Chosen: true}}
	cred := &fakeJiraCredentialForHandler{} // no credential stored
	h := NewSettingsHandler(&fakeUserDashboardService{}, cred, settings, SessionTarget, listener)

	// same switch value, nothing muted, board already on default, no credential
	_, c := newTestContextWithUser("PUT", "/x", `{"enabled":true}`)
	h.UpdateNotificationsSwitch(c)
	_, c = newTestContextWithUser("DELETE", "/x?board=28", "")
	h.Unmute(c)
	_, c = newTestContextWithUser("DELETE", "/x?board=28", "")
	h.ResetNotification(c)
	_, c = newTestContextWithUser("DELETE", "/x", "")
	h.DeleteCredential(c)
	// the default scope saved with its current values
	_, c = newTestContextWithUser("PUT", "/x", `{"reminderTiers":["2h","1h","30m","15m","5m","1m"],"overageTiers":[],"overageRepeatMinutes":5,"notifyStatusChange":true,"statusChangeStatuses":[],"quietHours":null}`)
	h.UpdateNotification(c)

	if len(changes) != 5 {
		t.Fatalf("got %d changes, want 5", len(changes))
	}
	for _, rc := range changes {
		if !rc.change.NoOp {
			t.Errorf("%s: NoOp = false, want true", rc.change.Kind)
		}
	}

	// a real change is not a no-op: switch flipped, tiers changed
	changes = nil
	_, c = newTestContextWithUser("PUT", "/x", `{"enabled":false}`)
	h.UpdateNotificationsSwitch(c)
	_, c = newTestContextWithUser("PUT", "/x", `{"reminderTiers":["1h"],"overageTiers":[],"overageRepeatMinutes":5,"notifyStatusChange":true,"quietHours":null}`)
	h.UpdateNotification(c)
	if len(changes) != 2 || changes[0].change.NoOp || changes[1].change.NoOp {
		t.Fatalf("changes = %+v, want two real changes", changes)
	}
	if changes[0].change.Enabled == nil || *changes[0].change.Enabled || changes[1].change.Before == nil || changes[1].change.After == nil {
		t.Errorf("details missing: %+v", changes)
	}
}
