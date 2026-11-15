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

// The app is served from the root of its own subdomain by default; a
// prefix (or "/") from the environment is normalized without the trailing slash.
func TestLoadConfig_BasePath(t *testing.T) {
	cases := map[string]string{"": "", "/": "", "/nudge": "/nudge", "/nudge/": "/nudge"}
	for env, want := range cases {
		t.Setenv("BASE_PATH", env)
		if env == "" {
			os.Unsetenv("BASE_PATH")
		}
		var cfg AppConfig
		if err := LoadConfig(&cfg); err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}
		if cfg.BasePath != want {
			t.Errorf("BASE_PATH=%q → BasePath %q, want %q", env, cfg.BasePath, want)
		}
	}
}

func TestLoadConfig_ReadsFromDotEnvFile(t *testing.T) {
	dir := t.TempDir()
	envContents := "JIRA_BASE_URL=https://esb-id.atlassian.net\n" +
		"JIRA_EMAIL=bot@esb.co.id\n" +
		"JIRA_API_TOKEN=secret-token\n" +
		"GOOGLE_CHAT_WEBHOOK_URL=https://chat.googleapis.com/v1/spaces/XXX/messages\n" +
		"SQLITE_PATH=./data/test.db\n" +
		"HOLIDAY_CALENDAR_PATH=./config/holidays_id.json\n"
	if err := os.WriteFile(dir+"/.env", []byte(envContents), 0644); err != nil {
		t.Fatalf("failed to write temp .env file: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir into temp dir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalWD) })

	var cfg AppConfig
	if err := LoadConfig(&cfg); err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if cfg.HolidayCalendarPath != "./config/holidays_id.json" {
		t.Errorf("HolidayCalendarPath = %q, want %q — LoadConfig must read values from a .env file on disk, not just OS environment variables", cfg.HolidayCalendarPath, "./config/holidays_id.json")
	}
	if cfg.JiraBaseURL != "https://esb-id.atlassian.net" {
		t.Errorf("JiraBaseURL = %q, want %q", cfg.JiraBaseURL, "https://esb-id.atlassian.net")
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

func TestLoadConfig_ChatAppSettings(t *testing.T) {
	t.Setenv("GOOGLE_CHAT_APP_CREDENTIALS_FILE", "/var/www/nudge/shared/data/chat-app-sa.json")
	t.Setenv("GOOGLE_CHAT_APP_PROJECT_NUMBER", "275006361614")
	t.Setenv("GOOGLE_CHAT_APP_ACTIVATION_REDIRECT_URL", "https://nudge.labcup.tech/auth/chat-app/callback")

	var cfg AppConfig
	if err := LoadConfig(&cfg); err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.GoogleChatAppCredentialsFile != "/var/www/nudge/shared/data/chat-app-sa.json" {
		t.Errorf("GoogleChatAppCredentialsFile = %q", cfg.GoogleChatAppCredentialsFile)
	}
	if cfg.GoogleChatAppProjectNumber != "275006361614" {
		t.Errorf("GoogleChatAppProjectNumber = %q", cfg.GoogleChatAppProjectNumber)
	}
	if cfg.GoogleChatAppActivationRedirectURL != "https://nudge.labcup.tech/auth/chat-app/callback" {
		t.Errorf("GoogleChatAppActivationRedirectURL = %q", cfg.GoogleChatAppActivationRedirectURL)
	}
	if !cfg.GoogleChatAppOneClick {
		t.Error("GoogleChatAppOneClick = false, want true by default")
	}

	t.Setenv("GOOGLE_CHAT_APP_ONE_CLICK", "false")
	if err := LoadConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.GoogleChatAppOneClick {
		t.Error("GoogleChatAppOneClick = true, want false when GOOGLE_CHAT_APP_ONE_CLICK=false")
	}
}
