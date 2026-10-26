package config

import (
	"sync"

	"github.com/spf13/viper"
)

type AppConfig struct {
	JiraBaseURL          string
	JiraEmail            string
	JiraAPIToken         string
	GoogleChatWebhookURL string
	SQLitePath           string
	HolidayCalendarPath  string
	Port                 string
}

func LoadConfig(cfg *AppConfig) error {
	v := viper.New()
	v.AutomaticEnv()
	v.SetDefault("PORT", "8080")

	cfg.JiraBaseURL = v.GetString("JIRA_BASE_URL")
	cfg.JiraEmail = v.GetString("JIRA_EMAIL")
	cfg.JiraAPIToken = v.GetString("JIRA_API_TOKEN")
	cfg.GoogleChatWebhookURL = v.GetString("GOOGLE_CHAT_WEBHOOK_URL")
	cfg.SQLitePath = v.GetString("SQLITE_PATH")
	cfg.HolidayCalendarPath = v.GetString("HOLIDAY_CALENDAR_PATH")
	cfg.Port = v.GetString("PORT")

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
