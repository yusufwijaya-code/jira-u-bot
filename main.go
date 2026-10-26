package main

import (
	"database/sql"
	"log"

	"jira-u-bot/base/helpers/shutdown"
	"jira-u-bot/config"
	"jira-u-bot/migrations"
	"jira-u-bot/modules/business_hours/business_hours_service"
	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_service"
	"jira-u-bot/modules/issue_session/issue_session_repository"
	"jira-u-bot/modules/issue_session/issue_session_service"
	"jira-u-bot/modules/jira_client/jira_client_service"
	"jira-u-bot/modules/poller/poller_service"
	"jira-u-bot/modules/registration/registration_repository"
	"jira-u-bot/modules/reminder_engine/reminder_engine_service"
	"jira-u-bot/modules/reminder_log/reminder_log_repository"
	"jira-u-bot/modules/reminder_log/reminder_log_service"

	_ "modernc.org/sqlite"
)

func main() {
	var cfg config.AppConfig
	if err := config.LoadConfig(&cfg); err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	db, err := sql.Open("sqlite", cfg.SQLitePath)
	if err != nil {
		log.Fatalf("failed to open sqlite database at %s: %v", cfg.SQLitePath, err)
	}
	defer db.Close()

	if err := migrations.Apply(db); err != nil {
		log.Fatalf("failed to apply migrations: %v", err)
	}

	businessHoursSvc := business_hours_service.NewBusinessHoursService()
	holidayCalendar, err := businessHoursSvc.LoadHolidayCalendar(cfg.HolidayCalendarPath)
	if err != nil {
		log.Fatalf("failed to load holiday calendar from %s: %v", cfg.HolidayCalendarPath, err)
	}

	issueSessionRepo := issue_session_repository.NewIssueSessionRepository(db)
	issueSessionSvc := issue_session_service.NewIssueSessionService(issueSessionRepo)

	reminderLogRepo := reminder_log_repository.NewReminderLogRepository(db)
	reminderLogSvc := reminder_log_service.NewReminderLogService(reminderLogRepo)

	registrationRepo := registration_repository.NewRegistrationRepository(db)

	jiraClientSvc := jira_client_service.NewJiraClientService(&cfg)

	notifierSvc := google_chat_notifier_service.NewGoogleChatNotifierService(&cfg)

	reminderEngineSvc := reminder_engine_service.NewReminderEngineService(
		cfg.JiraBaseURL, holidayCalendar, businessHoursSvc, reminderLogSvc,
		issueSessionSvc, registrationRepo, notifierSvc,
	)

	pollerSvc := poller_service.NewPollerService(
		jiraClientSvc, issueSessionSvc, businessHoursSvc, reminderEngineSvc, holidayCalendar,
	)

	shutdownMgr := shutdown.NewManager()
	shutdownMgr.AddWorker()
	go func() {
		defer shutdownMgr.WorkerDone()
		pollerSvc.Run(shutdownMgr.Context())
	}()

	router := SetupRouter(db)

	log.Printf("jira-u-bot listening on :%s", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
