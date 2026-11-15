package admin_dashboard_service

import (
	"context"
	"fmt"
	"log"
	"time"

	"nudge/modules/admin_dashboard/admin_dashboard_dto"
	"nudge/modules/board/board_dto"
	"nudge/modules/user_setting/user_setting_dto"
)

// noticeTimeout bounds one change-notice DM, which runs detached from the
// admin's request.
const noticeTimeout = 10 * time.Second

// BoardNamer looks up a cached board's name — board_service.BoardService.
type BoardNamer interface {
	Get(ctx context.Context, boardID int64) (board_dto.Board, bool, error)
}

// Option configures optional dependencies of the admin dashboard service.
type Option func(*adminDashboardServiceImpl)

// WithBoardNamer lets change notices name the board instead of "#<id>".
func WithBoardNamer(b BoardNamer) Option {
	return func(s *adminDashboardServiceImpl) { s.boards = b }
}

var shortMonth = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func scopeLabel(boardName string, boardID int64) string {
	if boardID == user_setting_dto.DefaultBoardID {
		return "default semua board"
	}
	return "board " + boardName
}

// ChangeNoticeText is the DM a user gets when an admin changes their
// settings (Tech Spec "Admin Panel Revamp", AC4.1).
func ChangeNoticeText(adminEmail, boardName string, ch admin_dashboard_dto.UserChange) string {
	scope := scopeLabel(boardName, ch.BoardID)
	switch ch.Kind {
	case "notification":
		return fmt.Sprintf("Admin %s mengubah pengaturan notifikasi kamu (%s).", adminEmail, scope)
	case "reset":
		return fmt.Sprintf("Admin %s mengembalikan pengaturan %s kamu ke default.", adminEmail, scope)
	case "mute":
		if ch.Until == nil || user_setting_dto.IsForever(ch.Until) {
			return fmt.Sprintf("Admin %s membungkam notifikasi kamu (%s) selamanya.", adminEmail, scope)
		}
		when := ch.Until.In(user_setting_dto.Jakarta)
		return fmt.Sprintf("Admin %s membungkam notifikasi kamu (%s) sampai %d %s %02d.%02d WIB.",
			adminEmail, scope, when.Day(), shortMonth[when.Month()-1], when.Hour(), when.Minute())
	case "unmute":
		return fmt.Sprintf("Admin %s mengaktifkan kembali notifikasi kamu (%s).", adminEmail, scope)
	case "credential_updated":
		return fmt.Sprintf("Admin %s memperbarui Jira credential kamu.", adminEmail)
	case "credential_deleted":
		return fmt.Sprintf("Admin %s menghapus Jira credential kamu.", adminEmail)
	case "chat_sender":
		return fmt.Sprintf("Admin %s mengubah pengirim pesan Nudge kamu.", adminEmail)
	}
	return fmt.Sprintf("Admin %s mengubah pengaturan kamu.", adminEmail)
}

// NotifyUserChanged DMs targetEmail about an admin's change — best-effort:
// never for the admin's own account, never blocks the request, errors are
// only logged (AC4.2, AC4.3).
func (s *adminDashboardServiceImpl) NotifyUserChanged(actingUserID int64, actingEmail string, targetUserID int64, targetEmail string, ch admin_dashboard_dto.UserChange) {
	if s.dmNotifier == nil || actingUserID == targetUserID {
		return
	}
	send := func() {
		ctx, cancel := context.WithTimeout(context.Background(), noticeTimeout)
		defer cancel()
		boardName := fmt.Sprintf("#%d", ch.BoardID)
		if ch.BoardID != user_setting_dto.DefaultBoardID && s.boards != nil {
			if b, found, err := s.boards.Get(ctx, ch.BoardID); err == nil && found && b.Name != "" {
				boardName = b.Name
			}
		}
		if err := s.dmNotifier.SendPlainTextDM(ctx, targetEmail, ChangeNoticeText(actingEmail, boardName, ch)); err != nil {
			log.Printf("admin_dashboard_service: change notice DM to %s failed: %v", targetEmail, err)
		}
	}
	if s.sendSync {
		send()
		return
	}
	go send()
}
