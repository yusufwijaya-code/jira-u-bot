package admin_dashboard_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"nudge/modules/admin_dashboard/admin_dashboard_dto"
	"nudge/modules/board/board_dto"
	"nudge/modules/user_setting/user_setting_dto"
)

func TestChangeNoticeText(t *testing.T) {
	until := time.Date(2026, 11, 14, 16, 30, 0, 0, user_setting_dto.Jakarta)
	forever := user_setting_dto.MuteForeverUntil
	cases := []struct {
		name  string
		board string
		ch    admin_dashboard_dto.UserChange
		want  string
	}{
		{"notif default", "", admin_dashboard_dto.UserChange{Kind: "notification"}, "Admin a@esb.co.id mengubah pengaturan notifikasi kamu (default semua board)."},
		{"notif board", "ESB Core Board", admin_dashboard_dto.UserChange{Kind: "notification", BoardID: 5}, "Admin a@esb.co.id mengubah pengaturan notifikasi kamu (board ESB Core Board)."},
		{"reset", "ESB Core Board", admin_dashboard_dto.UserChange{Kind: "reset", BoardID: 5}, "Admin a@esb.co.id mengembalikan pengaturan board ESB Core Board kamu ke default."},
		{"mute", "", admin_dashboard_dto.UserChange{Kind: "mute", Until: &until}, "Admin a@esb.co.id membungkam notifikasi kamu (default semua board) sampai 14 Nov 16.30 WIB."},
		{"mute forever", "", admin_dashboard_dto.UserChange{Kind: "mute", Until: &forever}, "Admin a@esb.co.id membungkam notifikasi kamu (default semua board) selamanya."},
		{"unmute", "X", admin_dashboard_dto.UserChange{Kind: "unmute", BoardID: 5}, "Admin a@esb.co.id mengaktifkan kembali notifikasi kamu (board X)."},
		{"cred up", "", admin_dashboard_dto.UserChange{Kind: "credential_updated"}, "Admin a@esb.co.id memperbarui Jira credential kamu."},
		{"cred del", "", admin_dashboard_dto.UserChange{Kind: "credential_deleted"}, "Admin a@esb.co.id menghapus Jira credential kamu."},
	}
	for _, tc := range cases {
		if got := ChangeNoticeText("a@esb.co.id", tc.board, tc.ch); got != tc.want {
			t.Errorf("%s:\n got  %q\n want %q", tc.name, got, tc.want)
		}
	}
}

type fakeBoardNamer struct{ boards map[int64]string }

func (f fakeBoardNamer) Get(ctx context.Context, boardID int64) (board_dto.Board, bool, error) {
	name, ok := f.boards[boardID]
	return board_dto.Board{BoardID: boardID, Name: name}, ok, nil
}

func syncService(dm *fakeChatDMNotifierForAdmin, opts ...Option) *adminDashboardServiceImpl {
	var notifier = dm
	svc := NewAdminDashboardService(&fakeRegistrationRepoForAdmin{}, &fakeJiraCredentialForAdmin{}, notifier, "", opts...).(*adminDashboardServiceImpl)
	svc.sendSync = true
	return svc
}

func TestNotifyUserChanged_SkipsSelf(t *testing.T) {
	dm := &fakeChatDMNotifierForAdmin{}
	syncService(dm).NotifyUserChanged(1, "a@esb.co.id", 1, "a@esb.co.id", admin_dashboard_dto.UserChange{Kind: "notification"})
	if len(dm.sentTo) != 0 {
		t.Errorf("self change sent %d DMs, want 0", len(dm.sentTo))
	}
}

func TestNotifyUserChanged_SendsWithBoardName(t *testing.T) {
	dm := &fakeChatDMNotifierForAdmin{}
	svc := syncService(dm, WithBoardNamer(fakeBoardNamer{boards: map[int64]string{5: "ESB Core Board"}}))
	svc.NotifyUserChanged(1, "a@esb.co.id", 2, "b@esb.co.id", admin_dashboard_dto.UserChange{Kind: "unmute", BoardID: 5})
	want := "Admin a@esb.co.id mengaktifkan kembali notifikasi kamu (board ESB Core Board)."
	if dm.sentText["b@esb.co.id"] != want {
		t.Errorf("DM = %q, want %q", dm.sentText["b@esb.co.id"], want)
	}
}

func TestNotifyUserChanged_UnknownBoardFallsBackToID(t *testing.T) {
	dm := &fakeChatDMNotifierForAdmin{}
	syncService(dm).NotifyUserChanged(1, "a@esb.co.id", 2, "b@esb.co.id", admin_dashboard_dto.UserChange{Kind: "reset", BoardID: 77})
	want := "Admin a@esb.co.id mengembalikan pengaturan board #77 kamu ke default."
	if dm.sentText["b@esb.co.id"] != want {
		t.Errorf("DM = %q, want %q", dm.sentText["b@esb.co.id"], want)
	}
}

func TestNotifyUserChanged_DMErrorIsSwallowed(t *testing.T) {
	dm := &fakeChatDMNotifierForAdmin{failEmails: map[string]error{"b@esb.co.id": errors.New("chat down")}}
	syncService(dm).NotifyUserChanged(1, "a@esb.co.id", 2, "b@esb.co.id", admin_dashboard_dto.UserChange{Kind: "credential_deleted"})
	// no panic, nothing to assert beyond reaching here
}

func TestNotifyUserChanged_NilNotifierIsNoop(t *testing.T) {
	svc := NewAdminDashboardService(&fakeRegistrationRepoForAdmin{}, &fakeJiraCredentialForAdmin{}, nil, "")
	svc.NotifyUserChanged(1, "a@esb.co.id", 2, "b@esb.co.id", admin_dashboard_dto.UserChange{Kind: "mute"})
}

func TestChangeNoticeText_ChatSender(t *testing.T) {
	if got := ChangeNoticeText("admin@esb.co.id", "", admin_dashboard_dto.UserChange{Kind: "chat_sender"}); got != "Admin admin@esb.co.id mengubah pengirim pesan Nudge kamu." {
		t.Errorf("chat_sender notice = %q", got)
	}
}
