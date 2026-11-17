package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"nudge/base/helpers/shutdown"
	"nudge/config"
	"nudge/migrations"
	"nudge/modules/chat_group"
	"nudge/modules/admin_dashboard/admin_dashboard_handler"
	"nudge/modules/admin_dashboard/admin_dashboard_service"
	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_handler"
	"nudge/modules/auth/auth_service"
	"nudge/modules/auth/auth_session_repository"
	"nudge/modules/board/board_repository"
	"nudge/modules/board/board_service"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/chat_app/chat_app_auth"
	"nudge/modules/chat_app/chat_app_card"
	"nudge/modules/chat_app/chat_app_client"
	"nudge/modules/chat_app/chat_app_handler"
	"nudge/modules/chat_app/chat_app_repository"
	"nudge/modules/chat_app/chat_app_service"
	"nudge/modules/chat_app_activation"
	"nudge/modules/chat_command_poll_cursor/chat_command_poll_cursor_repository"
	"nudge/modules/chat_command_poller/chat_command_poller_service"
	"nudge/modules/chat_delivery"
	"nudge/modules/chat_dm_notifier/chat_dm_notifier_service"
	"nudge/modules/chat_interactive/chat_interactive_service"
	"nudge/modules/chat_log"
	"nudge/modules/board_scope"
	"nudge/modules/chat_log/chat_log_handler"
	"nudge/modules/chat_log/chat_log_repository"
	"nudge/modules/chat_oauth/chat_oauth_service"
	"nudge/modules/gemini_client"
	"nudge/modules/issue_create/issue_create_handler"
	"nudge/modules/issue_create/issue_create_service"
	"nudge/modules/issue_session/issue_session_repository"
	"nudge/modules/issue_session/issue_session_service"
	"nudge/modules/jira_client/jira_client_service"
	"nudge/modules/jira_credential/jira_credential_repository"
	"nudge/modules/jira_credential/jira_credential_service"
	"nudge/modules/move_fields"
	"nudge/modules/poller/poller_service"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/registration/registration_repository"
	"nudge/modules/registration/registration_service"
	"nudge/modules/reminder_engine/reminder_engine_service"
	"nudge/modules/reminder_log/reminder_log_repository"
	"nudge/modules/reminder_log/reminder_log_service"
	"nudge/modules/sprint_backfill/sprint_backfill_service"
	"nudge/modules/sprint_report/sprint_report_handler"
	"nudge/modules/sprint_report/sprint_report_service"
	"nudge/modules/status_watch/status_watch_repository"
	"nudge/modules/status_watch/status_watch_service"
	"nudge/modules/tracking_rule"
	"nudge/modules/user_avatar/user_avatar_handler"
	"nudge/modules/user_avatar/user_avatar_repository"
	"nudge/modules/user_avatar/user_avatar_service"
	"nudge/modules/user_dashboard/user_dashboard_handler"
	"nudge/modules/user_dashboard/user_dashboard_service"
	"nudge/modules/user_leave/user_leave_handler"
	"nudge/modules/user_leave/user_leave_repository"
	"nudge/modules/user_leave/user_leave_service"
	"nudge/modules/user_setting/user_setting_repository"
	"nudge/modules/user_setting/user_setting_service"

	_ "github.com/go-sql-driver/mysql"
)

// setupMode returns the one-time OAuth consent flow requested on the
// command line ("" = normal start).
func setupMode(args []string) string {
	for _, a := range args[1:] {
		if a == "--setup-oauth" || a == "--setup-groups-oauth" {
			return a
		}
	}
	return ""
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

// chatLogRetention is how long the admin "Riwayat pesan" keeps entries.
const chatLogRetention = 90 * 24 * time.Hour

func main() {
	if mode := setupMode(os.Args); mode != "" {
		var cfg config.AppConfig
		if err := config.LoadConfig(&cfg); err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
		run := chat_oauth_service.RunSetupFlow
		if mode == "--setup-groups-oauth" {
			run = chat_group.RunSetupFlow
		}
		if err := run(&cfg); err != nil {
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

	// Tech Spec "Papan Tiket & Cuti": every time calculation uses the
	// assignee's own calendar (national holidays + their leave days). A
	// change to a past leave day rebuilds that user's tracked time from
	// Jira; recalcLeave is set once sprintBackfillSvc exists below.
	var recalcLeave func(email string)
	userLeaveSvc := user_leave_service.NewUserLeaveService(
		user_leave_repository.NewUserLeaveRepository(db), holidayCalendar,
		func(email string) {
			if recalcLeave != nil {
				recalcLeave(email)
			}
		},
	)

	issueSessionRepo := issue_session_repository.NewIssueSessionRepository(db)
	issueSessionSvc := issue_session_service.NewIssueSessionService(issueSessionRepo)

	reminderLogRepo := reminder_log_repository.NewReminderLogRepository(db)
	reminderLogSvc := reminder_log_service.NewReminderLogService(reminderLogRepo)

	registrationRepo := registration_repository.NewRegistrationRepository(db)

	jiraAgileClient := jira_client_service.NewJiraAgileClient(&cfg)
	boardSvc := board_service.NewBoardService(board_repository.NewBoardRepository(db), jiraAgileClient)

	userSettingRepo := user_setting_repository.NewUserSettingRepository(db)
	userSettingSvc := user_setting_service.NewUserSettingService(userSettingRepo, registrationRepo)

	jiraClientSvc := jira_client_service.NewJiraClientService(&cfg)

	jiraCredentialRepo := jira_credential_repository.NewJiraCredentialRepository(db)
	jiraCredentialSvc := jira_credential_service.NewJiraCredentialService(&cfg, jiraCredentialRepo, registrationRepo)

	userDashboardSvc := user_dashboard_service.NewUserDashboardService(
		jiraClientSvc, jiraCredentialSvc, registrationRepo, issueSessionRepo, businessHoursSvc, userLeaveSvc, cfg.JiraBaseURL, jiraAgileClient, boardSvc,
		// Moves collect the fields Jira requires first (workflow validators,
		// transition screens, Jira's 400) — automatic for every board.
		user_dashboard_service.WithMoveFields(move_fields.New(jira_client_service.NewTransitionFieldsClient(&cfg))),
	)

	issueCreateSvc := issue_create_service.New(issue_create_service.Deps{
		Jira:        jira_client_service.NewIssueCreateClient(&cfg),
		Credentials: jiraCredentialSvc,
		Boards:      userDashboardSvc,
		AI:          gemini_client.New(cfg.GeminiAPIKey, cfg.GeminiModel),
	})

	// Chat commands are the web dashboard's features (Dashboard, Papan Tiket,
	// Cuti, Mute) on the same services.
	boardScopeRepo := board_scope.New(db)
	chatInteractiveSvc := chat_interactive_service.New(chat_interactive_service.Deps{
		Users: registrationRepo, Dashboard: userDashboardSvc, Leaves: userLeaveSvc, Settings: userSettingSvc,
		Scope: boardScopeRepo,
		WebURL: cfg.FrontendURL + cfg.BasePath,
	})

	chatOAuthSvc, err := chat_oauth_service.NewChatOAuthService(&cfg)
	if err != nil {
		log.Fatalf("failed to initialize chat oauth service: %v", err)
	}

	// Admin "Riwayat pesan": every message Nudge sends and every command or
	// Chat event it receives, kept for chatLogRetention.
	chatLogRepo := chat_log_repository.New(db)

	var chatDMNotifierSvc chat_dm_notifier_service.ChatDMNotifierService
	var chatCommandPollerSvc chat_command_poller_service.ChatCommandPollerService
	if chatOAuthSvc != nil {
		chatDMNotifierSvc = chat_dm_notifier_service.NewChatDMNotifierService(chatOAuthSvc, registrationRepo)

		cursorRepo := chat_command_poll_cursor_repository.NewChatCommandPollCursorRepository(db)
		chatCommandPollerSvc = chat_command_poller_service.NewChatCommandPollerService(
			chatOAuthSvc, registrationRepo, cursorRepo,
			chat_log.LoggedCommands{ChatInteractiveService: chatInteractiveSvc, Log: chatLogRepo},
			chat_log.LoggedReplies{Inner: chatDMNotifierSvc, Log: chatLogRepo},
			cfg.GoogleChatSelfUserID,
		)
	}

	// Tech Spec "Nudge sebagai Google Chat App": every message goes through
	// chat_delivery, which picks the recipient's sender (Nudge app or the
	// operator DM) and falls back to the operator DM when the app fails.
	chatAppRepo := chat_app_repository.NewRepository(db)
	var chatAppClient chat_app_client.Client
	if cfg.GoogleChatAppCredentialsFile != "" {
		if c, err := chat_app_client.NewFromFile(cfg.GoogleChatAppCredentialsFile); err != nil {
			log.Printf("main: Chat App disabled: %v", err)
		} else {
			chatAppClient = c
		}
	}
	operatorName := resolveOperatorName(context.Background(), registrationRepo, cfg.JiraEmail)
	deliveryDeps := chat_delivery.Deps{Store: chatAppRepo, OperatorEmail: cfg.JiraEmail, OperatorName: operatorName, Log: chatLogRepo}
	deliveryDeps.Cards = chat_app_card.Build // phase 3: cards with action buttons on the app path
	deliveryDeps.TextCards = chat_app_card.TextCard // the "/n-help" hint of a notice becomes a menu button
	if chatAppClient != nil {
		deliveryDeps.App = chatAppClient
	}
	if chatDMNotifierSvc != nil {
		deliveryDeps.Legacy = chatDMNotifierSvc
	}
	deliveryRouter := chat_delivery.New(deliveryDeps)

	// Every notice goes through the router; the webhook space is no longer
	// used (Tech Spec "Nudge sebagai Google Chat App", AC1.6).
	reminderEngineSvc := reminder_engine_service.NewReminderEngineService(
		cfg.JiraBaseURL, userLeaveSvc, businessHoursSvc, reminderLogSvc,
		issueSessionSvc, registrationRepo, deliveryRouter, userSettingSvc,
	)

	pollerSvc := poller_service.NewPollerService(
		jiraClientSvc, issueSessionSvc, businessHoursSvc, reminderEngineSvc, registrationRepo, userLeaveSvc, boardSvc,
	)

	// Tech Spec "Active Sprint Tracking & Changelog Backfill": rebuild the
	// time of active-sprint tickets the poller never saw — at start and
	// every day at 07:00 WIB, plus the Sprint card's button.
	sprintBackfillSvc := sprint_backfill_service.NewSprintBackfillService(
		jiraAgileClient, registrationRepo, jiraCredentialSvc,
		issue_session_repository.NewSessionBackfillRepository(db), businessHoursSvc, userLeaveSvc,
	)
	recalcLeave = func(email string) {
		n, err := sprintBackfillSvc.RecalculateForEmail(context.Background(), email)
		if err != nil {
			log.Printf("main: leave recalculation of %s failed: %v", email, err)
			return
		}
		log.Printf("main: leave recalculation of %s rewrote %d session(s)", email, n)
	}

	// Tech Spec "Aturan Board (Admin CRUD)": tracking rules come from
	// ms_tracking_rule — loaded before the poller and the backfill start,
	// refreshed on every admin save and every 5 minutes.
	shutdownMgr := shutdown.NewManager()
	trackingRuleSvc := tracking_rule.NewService(tracking_rule.Deps{
		Store:    tracking_rule.NewRepository(db),
		Jira:     jira_client_service.NewProjectMetaClient(&cfg),
		Boards:   boardSvc,
		Recalc:   sprintBackfillSvc,
		Lifetime: shutdownMgr.Context(),
	})
	if err := trackingRuleSvc.Reload(context.Background()); err != nil {
		log.Fatalf("failed to load tracking rules: %v", err)
	}

	shutdownMgr.AddWorker()
	go func() {
		defer shutdownMgr.WorkerDone()
		pollerSvc.Run(shutdownMgr.Context())
	}()

	shutdownMgr.AddWorker()
	go func() {
		defer shutdownMgr.WorkerDone()
		sprintBackfillSvc.RunDaily(shutdownMgr.Context())
	}()

	shutdownMgr.AddWorker()
	go func() {
		defer shutdownMgr.WorkerDone()
		trackingRuleSvc.RunRefresh(shutdownMgr.Context())
	}()

	// Tech Spec "Notifikasi Perubahan Status Tiket": DM on every status
	// change of a registered user's active-sprint ticket (Jira or Nudge).
	statusWatchSvc := status_watch_service.NewStatusWatchService(status_watch_service.Deps{
		Jira: jiraAgileClient, Users: registrationRepo, Settings: userSettingSvc, Sessions: issueSessionRepo,
		Snapshots:     status_watch_repository.NewStatusWatchRepository(db),
		BusinessHours: businessHoursSvc, Calendar: userLeaveSvc,
		Notifier: deliveryRouter, JiraBaseURL: cfg.JiraBaseURL,
	})
	shutdownMgr.AddWorker()
	go func() {
		defer shutdownMgr.WorkerDone()
		statusWatchSvc.Run(shutdownMgr.Context())
	}()

	shutdownMgr.AddWorker()
	go func() {
		defer shutdownMgr.WorkerDone()
		chat_log.RunRetention(shutdownMgr.Context(), chatLogRepo, chatLogRetention)
	}()

	if chatCommandPollerSvc != nil {
		shutdownMgr.AddWorker()
		go func() {
			defer shutdownMgr.WorkerDone()
			chatCommandPollerSvc.Run(shutdownMgr.Context())
		}()
	}

	registrationService := registration_service.NewRegistrationService(jiraClientSvc, registrationRepo, deliveryRouter, cfg.JiraEmail)

	authSessionRepo := auth_session_repository.NewAuthSessionRepository(db)
	authSvc := auth_service.NewAuthService(&cfg, registrationService, registrationRepo, authSessionRepo)
	// Google profile pictures: stored from the Sign-In ID token on every
	// login, served through /me/avatar and the admin avatar endpoint.
	userAvatarSvc := user_avatar_service.NewUserAvatarService(user_avatar_repository.NewUserAvatarRepository(db), nil)
	// Auto Join Google Group (Tech Spec "Auto Join Google Group Nudge"): off
	// unless NUDGE_GROUP_EMAIL and the operator's Groups token are set.
	var chatGroupSvc *chat_group.Service
	if ts, on, err := chat_group.TokenSource(&cfg); err != nil {
		log.Printf("main: Groups token unreadable, auto join off: %v", err)
	} else if on {
		chatGroupSvc = chat_group.NewService(chat_group.NewClient(ts, cfg.NudgeGroupEmail, chat_group.DefaultBaseURL), chat_group.NewRepository(db))
		log.Printf("main: auto join Google Group %s on", cfg.NudgeGroupEmail)
	}
	authHandler := auth_handler.NewAuthHandler(authSvc, &cfg, auth_handler.WithLoginHook(
		func(ctx context.Context, userID int64, claims auth_dto.GoogleClaims) {
			if err := userAvatarSvc.SavePicture(ctx, userID, claims.Picture); err != nil {
				log.Printf("main: saving profile picture of user %d failed: %v", userID, err)
			}
			// Never delays or fails the login: runs on its own after the redirect.
			if chatGroupSvc != nil {
				email := claims.Email
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					chatGroupSvc.EnsureMember(ctx, email)
				}()
			}
		},
	))
	if chatGroupSvc != nil {
		shutdownMgr.AddWorker()
		go func() {
			defer shutdownMgr.WorkerDone()
			chatGroupSvc.Run(shutdownMgr.Context())
		}()
	}

	// The user's own changes on the web get a confirmation DM; chat commands
	// reply in chat instead.
	userDashboardHandler := user_dashboard_handler.NewUserDashboardHandler(userDashboardSvc, chatInteractiveSvc, jiraCredentialSvc, userSettingSvc, boardSvc,
		user_dashboard_handler.WithSelfChangeListener(user_dashboard_handler.SelfChangeListener(deliveryRouter, userDashboardSvc)))

	sprintReportSvc := sprint_report_service.NewSprintReportService(issueSessionRepo, businessHoursSvc, userLeaveSvc)
	sprintReportHandler := sprint_report_handler.NewSprintReportHandler(sprintReportSvc, sprint_report_handler.WithBackfiller(sprintBackfillSvc))

	adminDashboardSvc := admin_dashboard_service.NewAdminDashboardService(
		registrationRepo, jiraCredentialSvc, deliveryRouter, cfg.JiraEmail,
		admin_dashboard_service.WithBoardNamer(boardSvc),
		admin_dashboard_service.WithChatApp(chatAppRepo, chatAppClient != nil),
	)
	adminDashboardHandler := admin_dashboard_handler.NewAdminDashboardHandler(adminDashboardSvc)

	// Admin › User detail: the same settings endpoints, targeting :userID
	// (Tech Spec "Admin Panel Revamp").
	adminUserSettings := user_dashboard_handler.NewSettingsHandler(
		userDashboardSvc, jiraCredentialSvc, userSettingSvc, admin_dashboard_handler.AdminTarget(adminDashboardSvc),
		admin_dashboard_handler.AdminChangeListener(adminDashboardSvc),
	)

	userAvatarHandler := user_avatar_handler.NewUserAvatarHandler(userAvatarSvc)

	router := SetupRouter(&cfg, authSvc, authHandler, userDashboardHandler, adminDashboardHandler, sprintReportHandler, adminUserSettings, userAvatarHandler, user_leave_handler.NewUserLeaveHandler(userLeaveSvc))

	chatAppRoutes := ChatAppRoutes{}
	if chatAppClient != nil && cfg.GoogleChatAppProjectNumber != "" {
		chatAppSvc := chat_app_service.New(chat_app_service.Deps{
			Users: registrationRepo, States: chatAppRepo, Commands: chatInteractiveSvc, App: chatAppClient,
			Log: chatLogRepo,
			// Welcome card's "Nyalakan notifikasi" button. Registration stays web-only.
			Notifications: userSettingSvc,
		})
		chatAppRoutes.Events = chat_app_handler.New(chat_app_auth.NewVerifier(cfg.GoogleChatAppProjectNumber), chatAppSvc).Events
	} else if chatAppClient != nil {
		log.Printf("main: GOOGLE_CHAT_APP_PROJECT_NUMBER is empty — /api/chat/events not mounted")
	}
	var activationOAuth chat_app_activation.OAuth
	if cfg.GoogleChatAppOneClick && cfg.GoogleChatAppActivationRedirectURL != "" && cfg.GoogleSignInClientID != "" {
		activationOAuth = chat_app_activation.NewGoogleOAuth(cfg.GoogleSignInClientID, cfg.GoogleSignInClientSecret, cfg.GoogleChatAppActivationRedirectURL)
	}
	chatAppRoutes.Activation = chat_app_activation.New(chat_app_activation.Deps{
		States: chatAppRepo, OAuth: activationOAuth, Notify: deliveryRouter,
		AppEnabled: chatAppClient != nil, OneClick: cfg.GoogleChatAppOneClick, GroupEnabled: chatGroupSvc != nil,
		OperatorEmail: cfg.JiraEmail, OperatorName: operatorName,
		AppURL: cfg.FrontendURL + cfg.BasePath, CookiePath: cfg.BasePath,
	})
	chatAppRoutes.AdminTarget = admin_dashboard_handler.AdminTarget(adminDashboardSvc)
	chatAppRoutes.AdminChange = admin_dashboard_handler.AdminChangeListener(adminDashboardSvc)
	MountChatApp(router, &cfg, authSvc, chatAppRoutes)
	MountBoardScope(router, &cfg, authSvc, board_scope.NewHandler(boardScopeRepo))
	MountIssueCreate(router, &cfg, authSvc, issue_create_handler.New(issueCreateSvc, cfg.JiraBaseURL))
	MountChatLog(router, &cfg, authSvc, chat_log_handler.New(chatLogRepo, deliveryRouter, registrationRepo))
	MountTrackingRules(router, &cfg, authSvc, tracking_rule.NewHandler(trackingRuleSvc))

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

// resolveOperatorName is the operator account's name for "dikirim lewat
// akun <name>" footers — its ms_user name, or the email before "@".
func resolveOperatorName(ctx context.Context, users interface {
	FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error)
}, email string) string {
	if u, found, err := users.FindUserByEmail(ctx, email); err == nil && found && u.UserName != "" {
		return u.UserName
	}
	if i := strings.Index(email, "@"); i > 0 {
		return email[:i]
	}
	return "operator Nudge"
}
