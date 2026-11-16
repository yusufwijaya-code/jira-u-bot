package registration_service

import (
	"context"
	"errors"
	"fmt"
	"log"

	"nudge/modules/chat_delivery"
	"nudge/modules/chat_dm_notifier/chat_dm_notifier_service"
	"nudge/modules/jira_client/jira_client_service"
	"nudge/modules/registration/registration_repository"
)

type registrationServiceImpl struct {
	jiraClient jira_client_service.JiraClientService
	repo       registration_repository.RegistrationRepository
	dmNotifier chat_dm_notifier_service.ChatDMNotifierService
	selfEmail  string
}

func NewRegistrationService(
	jiraClient jira_client_service.JiraClientService,
	repo registration_repository.RegistrationRepository,
	dmNotifier chat_dm_notifier_service.ChatDMNotifierService,
	selfEmail string,
) RegistrationService {
	return &registrationServiceImpl{jiraClient: jiraClient, repo: repo, dmNotifier: dmNotifier, selfEmail: selfEmail}
}

func (s *registrationServiceImpl) Register(ctx context.Context, name, email string) error {
	accountID, found, err := s.jiraClient.LookupUserByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrJiraLookupUnavailable, err)
	}
	if !found {
		return ErrJiraAccountNotFound
	}

	if err := s.repo.UpsertUser(ctx, name, email, accountID); err != nil {
		return fmt.Errorf("registration_service: UpsertUser failed: %w", err)
	}

	s.sendThankYouDM(ctx, email)
	return nil
}

// sendThankYouDM is best-effort (spec AC3.2): registration is NOT affected
// by a failure here, and self-registration (email == selfEmail) never
// sends anything — DMing yourself was confirmed unsupported by the Google
// Chat API during live testing (spaces:setup rejects a self-membership
// with a 400 INVALID_ARGUMENT).
func (s *registrationServiceImpl) sendThankYouDM(ctx context.Context, email string) {
	if email == s.selfEmail || s.dmNotifier == nil {
		return
	}
	const text = `🎉 Halo! Registrasi kamu di Nudge berhasil.

🤖 Setiap tiket Code Process yang kamu pegang dipantau otomatis. Notifikasi ke DM ini masih *mati* — aktifkan lewat pop up setelah login di dashboard Nudge, atau kapan saja di menu *Pengaturan*.

Kalau notifikasi aktif, manfaatnya buat kamu:
⏰ Diingetin sebelum budget waktu habis (T-2 jam, T-1 jam, T-30 menit, T-15 menit, T-5 menit, T-1 menit)
🔴 Dapet notif kalau sudah lewat budget, biar bisa segera eskalasi
🎉 Dapet apresiasi kalau selesai lebih awal dari budget

Fokus ngerjain tiketnya aja, biar Nudge yang ingetin waktunya 💪

💡 Ketik */n-help* di sini buat lihat command lain`
	err := s.dmNotifier.SendPlainTextDM(ctx, email, text)
	switch {
	case errors.Is(err, chat_delivery.ErrRecipientNotActivated):
		// No sender can reach this user yet: skipped, not a failure.
		log.Printf("registration_service: thank-you DM to %s skipped: no chat sender available", email)
	case err != nil:
		log.Printf("registration_service: thank-you DM to %s failed: %v", email, err)
	}
}
