package config

import (
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
