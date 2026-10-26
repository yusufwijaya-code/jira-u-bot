package config

import (
	"os"
	"testing"
)

func TestLoadConfig_ReadsFromEnvironmentVariables(t *testing.T) {
	t.Setenv("JIRA_BASE_URL", "https://esb-id.atlassian.net")
	t.Setenv("JIRA_EMAIL", "bot@esb.co.id")
	t.Setenv("JIRA_API_TOKEN", "secret-token")
	t.Setenv("GOOGLE_CHAT_WEBHOOK_URL", "https://chat.googleapis.com/v1/spaces/XXX/messages")
	t.Setenv("SQLITE_PATH", "./data/test.db")
	t.Setenv("HOLIDAY_CALENDAR_PATH", "./config/holidays_id.json")
	t.Setenv("PORT", "9090")

	var cfg AppConfig
	if err := LoadConfig(&cfg); err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if cfg.JiraBaseURL != "https://esb-id.atlassian.net" {
		t.Errorf("JiraBaseURL = %q, want %q", cfg.JiraBaseURL, "https://esb-id.atlassian.net")
	}
	if cfg.JiraEmail != "bot@esb.co.id" {
		t.Errorf("JiraEmail = %q, want %q", cfg.JiraEmail, "bot@esb.co.id")
	}
	if cfg.JiraAPIToken != "secret-token" {
		t.Errorf("JiraAPIToken = %q, want %q", cfg.JiraAPIToken, "secret-token")
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
}

func TestLoadConfig_DefaultsPortWhenMissing(t *testing.T) {
	t.Setenv("JIRA_BASE_URL", "https://esb-id.atlassian.net")
	t.Setenv("JIRA_EMAIL", "bot@esb.co.id")
	t.Setenv("JIRA_API_TOKEN", "secret-token")
	t.Setenv("GOOGLE_CHAT_WEBHOOK_URL", "https://chat.googleapis.com/v1/spaces/XXX/messages")
	t.Setenv("SQLITE_PATH", "./data/test.db")
	t.Setenv("HOLIDAY_CALENDAR_PATH", "./config/holidays_id.json")
	os.Unsetenv("PORT")

	var cfg AppConfig
	if err := LoadConfig(&cfg); err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want default %q", cfg.Port, "8080")
	}
}

func TestGet_ReturnsSameInstanceAcrossCalls(t *testing.T) {
	t.Setenv("JIRA_BASE_URL", "https://esb-id.atlassian.net")
	t.Setenv("JIRA_EMAIL", "bot@esb.co.id")
	t.Setenv("JIRA_API_TOKEN", "secret-token")
	t.Setenv("GOOGLE_CHAT_WEBHOOK_URL", "https://chat.googleapis.com/v1/spaces/XXX/messages")
	t.Setenv("SQLITE_PATH", "./data/test.db")
	t.Setenv("HOLIDAY_CALENDAR_PATH", "./config/holidays_id.json")

	first := Get()
	second := Get()

	if first != second {
		t.Error("Get() returned different instances across calls, want singleton")
	}
}
