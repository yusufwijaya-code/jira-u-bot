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

	"jira-u-bot/base/helpers/shutdown"
	"jira-u-bot/config"
	"jira-u-bot/migrations"
	"jira-u-bot/modules/chat_dm_notifier/chat_dm_notifier_service"
	"jira-u-bot/modules/chat_interactive/chat_interactive_handler"
	"jira-u-bot/modules/chat_interactive/chat_interactive_middleware"
	"jira-u-bot/modules/chat_interactive/chat_interactive_service"
	"jira-u-bot/modules/chat_oauth/chat_oauth_service"
	"jira-u-bot/modules/business_hours/business_hours_service"
	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_service"
	"jira-u-bot/modules/issue_session/issue_session_repository"
	"jira-u-bot/modules/issue_session/issue_session_service"
	"jira-u-bot/modules/jira_client/jira_client_service"
	"jira-u-bot/modules/poller/poller_service"
	"jira-u-bot/modules/registration/registration_handler"
	"jira-u-bot/modules/registration/registration_repository"
	"jira-u-bot/modules/registration/registration_service"
	"jira-u-bot/modules/reminder_engine/reminder_engine_service"
	"jira-u-bot/modules/reminder_log/reminder_log_repository"
	"jira-u-bot/modules/reminder_log/reminder_log_service"

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

func mysqlDSN(cfg config.AppConfig) string {
	return fmt.Sprintf(
		"%s:%s@(%s:%s)/%s?parseTime=true&loc=Asia%%2FJakarta&multiStatements=true",
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

	chatInteractiveSvc := chat_interactive_service.NewChatInteractiveService(registrationRepo, issueSessionRepo, businessHoursSvc, holidayCalendar)
	chatInteractiveHandler := chat_interactive_handler.NewChatInteractiveHandler(chatInteractiveSvc)
	chatEventsVerifier := chat_interactive_middleware.NewGoogleTokenVerifier()

	jiraClientSvc := jira_client_service.NewJiraClientService(&cfg)

	notifierSvc := google_chat_notifier_service.NewGoogleChatNotifierService(&cfg)

	chatOAuthSvc, err := chat_oauth_service.NewChatOAuthService(&cfg)
	if err != nil {
		log.Fatalf("failed to initialize chat oauth service: %v", err)
	}

	var dmNotifierSvc google_chat_notifier_service.GoogleChatNotifierService
	var chatDMNotifierSvc chat_dm_notifier_service.ChatDMNotifierService
	if chatOAuthSvc != nil {
		chatDMNotifierSvc = chat_dm_notifier_service.NewChatDMNotifierService(chatOAuthSvc, registrationRepo)
		dmNotifierSvc = chatDMNotifierSvc
	}

	reminderEngineSvc := reminder_engine_service.NewReminderEngineService(
		cfg.JiraBaseURL, holidayCalendar, businessHoursSvc, reminderLogSvc,
		issueSessionSvc, registrationRepo, notifierSvc, dmNotifierSvc, cfg.JiraEmail,
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

	registrationService := registration_service.NewRegistrationService(jiraClientSvc, registrationRepo, chatDMNotifierSvc, cfg.JiraEmail)
	registrationHandler := registration_handler.NewRegistrationHandler(registrationService)
	router := SetupRouter(registrationHandler, chatInteractiveHandler, chatEventsVerifier, cfg.ChatEventsAudience)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	sigCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	go func() {
		<-sigCtx.Done()
		log.Println("jira-u-bot: shutdown signal received, stopping poller and http server")
		shutdownMgr.Shutdown()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("jira-u-bot: http server shutdown error: %v", err)
		}
	}()

	log.Printf("jira-u-bot listening on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server exited with error: %v", err)
	}

	shutdownMgr.Wait()
	log.Println("jira-u-bot: shutdown complete")
}
