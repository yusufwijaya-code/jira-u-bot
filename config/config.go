package config

import (
	"strings"
	"os"
	"sync"

	"github.com/spf13/viper"
)

type AppConfig struct {
	JiraBaseURL             string
	JiraEmail               string
	JiraAPIToken            string
	GoogleChatWebhookURL    string
	MySQLHost               string
	MySQLPort               string
	MySQLUser               string
	MySQLPassword           string
	MySQLDatabase           string
	GoogleOAuthClientID     string
	GoogleOAuthClientSecret string
	GoogleOAuthTokenPath    string
	HolidayCalendarPath     string
	Port                    string
	// GoogleChatSelfUserID is the Chat API sender resource name (format
	// "users/<id>") of the OAuth-authorized account itself — used by
	// chat_command_poller to skip its own sent messages when scanning a DM
	// space for incoming commands. Captured once by comparing
	// sender.name on a message the operator sent, e.g. via the Chat API.
	GoogleChatSelfUserID string
	// GoogleSignInClientID/Secret/RedirectURL are a SEPARATE OAuth Client
	// (type "Web application") from GoogleOAuthClientID/Secret above (type
	// "Desktop app", used for the DM-sending feature) — both live in the
	// same GCP project (Tech Spec "Dashboard Jira Manage", Open Question
	// #5), but Sign-In with Google needs its own redirect-based client.
	GoogleSignInClientID     string
	GoogleSignInClientSecret string
	GoogleSignInRedirectURL  string
	// AdminEmail is the single hardcoded admin account (Tech Spec
	// "Dashboard Jira Manage", AC1.6) — not a settable role, a constant
	// read from config so it's not duplicated across source files.
	AdminEmail string
	// CredentialEncryptionKey is the 32-byte base64 key used to encrypt
	// personal Jira API tokens at rest (tr_usercredential) — see
	// modules/jira_credential/jira_credential_crypto.
	CredentialEncryptionKey string
	// BasePath is the path prefix this entire app is served under —
	// labcup.tech hosts several repos behind the same nginx vhost, each
	// claiming its own prefix (nginx does NOT strip it before proxying,
	// matching the existing /nudge convention from the old
	// registration page) — every route, redirect, cookie Path, and
	// template link must be built relative to this, never hardcoded as a
	// root-absolute path.
	BasePath string
	// FrontendURL is the origin (scheme://host[:port], no trailing slash)
	// the Angular frontend is served from, used to build the redirects the
	// backend sends after login/logout. Empty (production, same origin
	// behind nginx) means relative redirects; in local dev set it to
	// http://localhost:4200 so the browser lands on the Angular dev server.
	FrontendURL string
}

func LoadConfig(cfg *AppConfig) error {
	v := viper.New()
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	// .env is optional — production deploys may set real OS env vars instead
	// (systemd/docker) with no .env file on disk at all.
	if _, statErr := os.Stat(".env"); statErr == nil {
		if err := v.ReadInConfig(); err != nil {
			return err
		}
	}
	v.AutomaticEnv()
	v.SetDefault("PORT", "8080")
	v.SetDefault("MYSQL_HOST", "localhost")
	v.SetDefault("MYSQL_PORT", "3308")
	v.SetDefault("MYSQL_USER", "root")
	v.SetDefault("MYSQL_DATABASE", "jira_db")
	v.SetDefault("GOOGLE_OAUTH_TOKEN_PATH", "./data/chat_oauth_token.json")
	v.SetDefault("BASE_PATH", "/nudge")

	cfg.JiraBaseURL = v.GetString("JIRA_BASE_URL")
	cfg.JiraEmail = v.GetString("JIRA_EMAIL")
	cfg.JiraAPIToken = v.GetString("JIRA_API_TOKEN")
	cfg.GoogleChatWebhookURL = v.GetString("GOOGLE_CHAT_WEBHOOK_URL")
	cfg.MySQLHost = v.GetString("MYSQL_HOST")
	cfg.MySQLPort = v.GetString("MYSQL_PORT")
	cfg.MySQLUser = v.GetString("MYSQL_USER")
	cfg.MySQLPassword = v.GetString("MYSQL_PASSWORD")
	cfg.MySQLDatabase = v.GetString("MYSQL_DATABASE")
	cfg.GoogleOAuthClientID = v.GetString("GOOGLE_OAUTH_CLIENT_ID")
	cfg.GoogleOAuthClientSecret = v.GetString("GOOGLE_OAUTH_CLIENT_SECRET")
	cfg.GoogleOAuthTokenPath = v.GetString("GOOGLE_OAUTH_TOKEN_PATH")
	cfg.HolidayCalendarPath = v.GetString("HOLIDAY_CALENDAR_PATH")
	cfg.Port = v.GetString("PORT")
	cfg.GoogleChatSelfUserID = v.GetString("GOOGLE_CHAT_SELF_USER_ID")
	cfg.GoogleSignInClientID = v.GetString("GOOGLE_SIGNIN_CLIENT_ID")
	cfg.GoogleSignInClientSecret = v.GetString("GOOGLE_SIGNIN_CLIENT_SECRET")
	cfg.GoogleSignInRedirectURL = v.GetString("GOOGLE_SIGNIN_REDIRECT_URL")
	cfg.AdminEmail = v.GetString("ADMIN_EMAIL")
	cfg.CredentialEncryptionKey = v.GetString("CREDENTIAL_ENCRYPTION_KEY")
	cfg.BasePath = v.GetString("BASE_PATH")
	cfg.FrontendURL = strings.TrimRight(v.GetString("FRONTEND_URL"), "/")

	return nil
}

var (
	instance     *AppConfig
	instanceOnce sync.Once
)

func Get() *AppConfig {
	instanceOnce.Do(func() {
		instance = &AppConfig{}
		_ = LoadConfig(instance)
	})
	return instance
}
