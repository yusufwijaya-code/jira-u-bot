package main

import (
	"strings"
	"testing"

	"jira-u-bot/config"
)

// TestMysqlDSN_SetsSessionTimeZoneToWIB reproduces a production bug: MySQL's
// CURRENT_TIMESTAMP (used by createdDate/editedDate/sentDate across several
// repositories) is evaluated by the MySQL SERVER's own session time_zone,
// which the DSN's parseTime/loc params do NOT influence (those only affect
// how the Go driver converts time.Time values it sends/receives itself).
// Confirmed live: the production server's time_zone is SYSTEM (UTC), while
// Jakarta is UTC+7, so those columns were coming back 7 hours behind wall-
// clock WIB. Explicitly setting the session time_zone via the DSN fixes it
// without touching the shared MySQL server's global config (that instance
// also hosts an unrelated database, fsldk_db).
func TestMysqlDSN_SetsSessionTimeZoneToWIB(t *testing.T) {
	dsn := mysqlDSN(config.AppConfig{
		MySQLUser: "root", MySQLPassword: "pw", MySQLHost: "localhost", MySQLPort: "3308", MySQLDatabase: "jira_db",
	})

	if !strings.Contains(dsn, "time_zone=%27%2B07%3A00%27") {
		t.Errorf("mysqlDSN() = %q, want it to set time_zone='+07:00' so CURRENT_TIMESTAMP evaluates in WIB", dsn)
	}
}

func TestShouldRunSetupMode_DetectsSetupOAuthFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"no args", []string{"jira-u-bot"}, false},
		{"unrelated flag", []string{"jira-u-bot", "--help"}, false},
		{"setup flag present", []string{"jira-u-bot", "--setup-oauth"}, true},
		{"setup flag among others", []string{"jira-u-bot", "-x", "--setup-oauth"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := shouldRunSetupMode(c.args)
			if got != c.want {
				t.Errorf("shouldRunSetupMode(%v) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}
