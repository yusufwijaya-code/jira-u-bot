package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nudge/base/helpers/shutdown"
	"nudge/config"
	"nudge/migrations"
	"nudge/modules/admin_dashboard/admin_dashboard_handler"
	"nudge/modules/admin_dashboard/admin_dashboard_service"
	"nudge/modules/auth/auth_handler"
	"nudge/modules/auth/auth_service"
	"nudge/modules/auth/auth_session_repository"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/chat_command_poll_cursor/chat_command_poll_cursor_repository"
	"nudge/modules/chat_command_poller/chat_command_poller_service"
	"nudge/modules/chat_dm_notifier/chat_dm_notifier_service"
	"nudge/modules/chat_interactive/chat_interactive_service"
	"nudge/modules/chat_oauth/chat_oauth_service"
	"nudge/modules/google_chat_notifier/google_chat_notifier_service"
	"nudge/modules/issue_session/issue_session_repository"
	"nudge/modules/issue_session/issue_session_service"
	"nudge/modules/jira_client/jira_client_service"
	"nudge/modules/jira_credential/jira_credential_repository"
	"nudge/modules/jira_credential/jira_credential_service"
	"nudge/modules/poller/poller_service"
	"nudge/modules/registration/registration_repository"
	"nudge/modules/registration/registration_service"
	"nudge/modules/reminder_engine/reminder_engine_service"
	"nudge/modules/reminder_log/reminder_log_repository"
	"nudge/modules/reminder_log/reminder_log_service"
	"nudge/modules/sprint_report/sprint_report_handler"
	"nudge/modules/sprint_report/sprint_report_service"
	"nudge/modules/user_dashboard/user_dashboard_handler"
	"nudge/modules/user_dashboard/user_dashboard_service"
	"nudge/modules/user_setting/user_setting_repository"
	"nudge/modules/user_setting/user_setting_service"

	_ "github.com/go-sql-driver/mysql"
)

func shouldRunSetupMode(args []string) bool {
	for _, a := range args[1:] {
		if a == "--setup-oauth" {
			return true
		}
	}
	return false
}

// mysqlDSN's time_zone param sets this connection's MySQL SESSION
// time_zone to WIB (+07:00) on every connect — scoped to this service's own
// connections only, unlike a server-wide `SET GLOBAL time_zone` (the shared
// MySQL instance also hosts an unrelated database, fsldk_db, that a global
// change would affect). Needed because CURRENT_TIMESTAMP (createdDate/
// editedDate/sentDate across several repositories) is evaluated by MySQL
// itself — parseTime/loc only affect how the Go driver converts time.Time
// values it sends/receives, not what CURRENT_TIMESTAMP resolves to.
func mysqlDSN(cfg config.AppConfig) string {
	return fmt.Sprintf(
		"%s:%s@(%s:%s)/%s?parseTime=true&loc=Asia%%2FJakarta&multiStatements=true&time_zone=%%27%%2B07%%3A00%%27",
		cfg.MySQLUser, cfg.MySQLPassword, cfg.MySQLHost, cfg.MySQLPort, cfg.MySQLDatabase,
	)
}

func main() {
	if shouldRunSetupMode(os.Args) {
		var cfg config.AppConfig
		if err := config.LoadConfig(&cfg); err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
		if err := chat_oauth_service.RunSetupFlow(&cfg); err != nil {
			log.Fatalf("oauth setup failed: %v", err)
		}
		return
	}

	var cfg config.AppConfig
	if err := config.LoadConfig(&cfg); err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	db, err := sql.Open("mysql", mysqlDSN(cfg))
	if err != nil {
		log.Fatalf("failed to open mysql connection: %v", err)
	}
	defer db.Close()
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(60 * time.Minute)
	db.SetConnMaxIdleTime(10 * time.Minute)
	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping mysql at %s:%s: %v", cfg.MySQLHost, cfg.MySQLPort, err)
	}

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

	userSettingRepo := user_setting_repository.NewUserSettingRepository(db)
	userSettingSvc := user_setting_service.NewUserSettingService(userSettingRepo, registrationRepo)

	jiraClientSvc := jira_client_service.NewJiraClientService(&cfg)

	jiraCredentialRepo := jira_credential_repository.NewJiraCredentialRepository(db)
	jiraCredentialSvc := jira_credential_service.NewJiraCredentialService(&cfg, jiraCredentialRepo, registrationRepo)

	chatInteractiveSvc := chat_interactive_service.NewChatInteractiveService(registrationRepo, issueSessionRepo, businessHoursSvc, holidayCalendar, jiraClientSvc, jiraCredentialSvc)

	notifierSvc := google_chat_notifier_service.NewGoogleChatNotifierService(&cfg)

	chatOAuthSvc, err := chat_oauth_service.NewChatOAuthService(&cfg)
	if err != nil {
		log.Fatalf("failed to initialize chat oauth service: %v", err)
	}

	var dmNotifierSvc google_chat_notifier_service.GoogleChatNotifierService
	var chatDMNotifierSvc chat_dm_notifier_service.ChatDMNotifierService
	var chatCommandPollerSvc chat_command_poller_service.ChatCommandPollerService
	if chatOAuthSvc != nil {
		chatDMNotifierSvc = chat_dm_notifier_service.NewChatDMNotifierService(chatOAuthSvc, registrationRepo)
		dmNotifierSvc = chatDMNotifierSvc

		cursorRepo := chat_command_poll_cursor_repository.NewChatCommandPollCursorRepository(db)
		chatCommandPollerSvc = chat_command_poller_service.NewChatCommandPollerService(
			chatOAuthSvc, registrationRepo, cursorRepo, chatInteractiveSvc, chatDMNotifierSvc, cfg.GoogleChatSelfUserID,
		)
	}

	reminderEngineSvc := reminder_engine_service.NewReminderEngineService(
		cfg.JiraBaseURL, holidayCalendar, businessHoursSvc, reminderLogSvc,
		issueSessionSvc, registrationRepo, notifierSvc, dmNotifierSvc, cfg.JiraEmail, userSettingSvc,
	)

	pollerSvc := poller_service.NewPollerService(
		jiraClientSvc, issueSessionSvc, businessHoursSvc, reminderEngineSvc, registrationRepo, holidayCalendar,
	)

	shutdownMgr := shutdown.NewManager()
	shutdownMgr.AddWorker()
	go func() {
		defer shutdownMgr.WorkerDone()
		pollerSvc.Run(shutdownMgr.Context())
	}()

	if chatCommandPollerSvc != nil {
		shutdownMgr.AddWorker()
		go func() {
			defer shutdownMgr.WorkerDone()
			chatCommandPollerSvc.Run(shutdownMgr.Context())
		}()
	}

	registrationService := registration_service.NewRegistrationService(jiraClientSvc, registrationRepo, chatDMNotifierSvc, cfg.JiraEmail)

	authSessionRepo := auth_session_repository.NewAuthSessionRepository(db)
	authSvc := auth_service.NewAuthService(&cfg, registrationService, registrationRepo, authSessionRepo)
	authHandler := auth_handler.NewAuthHandler(authSvc, &cfg)

	userDashboardSvc := user_dashboard_service.NewUserDashboardService(
		jiraClientSvc, jiraCredentialSvc, registrationRepo, issueSessionRepo, businessHoursSvc, holidayCalendar,
	)
	userDashboardHandler := user_dashboard_handler.NewUserDashboardHandler(userDashboardSvc, chatInteractiveSvc, jiraCredentialSvc, userSettingSvc)

	sprintReportSvc := sprint_report_service.NewSprintReportService(issueSessionRepo, businessHoursSvc, holidayCalendar)
	sprintReportHandler := sprint_report_handler.NewSprintReportHandler(sprintReportSvc)

	adminDashboardSvc := admin_dashboard_service.NewAdminDashboardService(registrationRepo, jiraCredentialSvc, chatDMNotifierSvc, cfg.JiraEmail)
	adminDashboardHandler := admin_dashboard_handler.NewAdminDashboardHandler(adminDashboardSvc)

	router := SetupRouter(&cfg, authSvc, authHandler, userDashboardHandler, adminDashboardHandler, sprintReportHandler)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	sigCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	go func() {
		<-sigCtx.Done()
		log.Println("nudge: shutdown signal received, stopping poller and http server")
		shutdownMgr.Shutdown()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("nudge: http server shutdown error: %v", err)
		}
	}()

	log.Printf("nudge listening on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server exited with error: %v", err)
	}

	shutdownMgr.Wait()
	log.Println("nudge: shutdown complete")
}
