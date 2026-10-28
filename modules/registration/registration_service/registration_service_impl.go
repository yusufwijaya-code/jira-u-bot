package registration_service

import (
	"context"
	"fmt"
	"log"

	"jira-u-bot/modules/chat_dm_notifier/chat_dm_notifier_service"
	"jira-u-bot/modules/jira_client/jira_client_service"
	"jira-u-bot/modules/registration/registration_repository"
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
	const text = `🎉 Halo! Registrasi kamu di Jira U Bot berhasil.

🤖 Mulai sekarang, setiap tiket Code Process yang kamu pegang bakal dipantau otomatis, dan reminder-nya masuk langsung ke DM ini.

Manfaatnya buat kamu:
⏰ Diingetin sebelum budget waktu habis (T-2 jam, T-1 jam, T-30 menit, T-15 menit, T-5 menit, T-1 menit)
🔴 Dapet notif kalau sudah lewat budget, biar bisa segera eskalasi
🎉 Dapet apresiasi kalau selesai lebih awal dari budget

Fokus ngerjain tiketnya aja, biar Jira U Bot yang ingetin waktunya 💪

💡 Ketik */u-help* di sini buat lihat command lain

#BersamaSayaMenempuhOKRyangPassss`
	if err := s.dmNotifier.SendPlainTextDM(ctx, email, text); err != nil {
		log.Printf("registration_service: thank-you DM to %s failed: %v", email, err)
	}
}
