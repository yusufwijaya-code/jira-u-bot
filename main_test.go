package main

import "testing"

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
