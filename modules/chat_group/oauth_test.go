package chat_group

import (
	"os"
	"path/filepath"
	"testing"

	"nudge/config"
)

func TestTokenSource_OffWhenGroupOrTokenMissing(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "groups.json")
	cfg := &config.AppConfig{GoogleOAuthClientID: "id", GoogleOAuthClientSecret: "s", GoogleGroupsTokenPath: tokenPath}

	if _, on, err := TokenSource(cfg); on || err != nil {
		t.Fatalf("no group email: on=%v err=%v, want off", on, err)
	}
	cfg.NudgeGroupEmail = "nudge-users@esb.co.id"
	if _, on, err := TokenSource(cfg); on || err != nil {
		t.Fatalf("no token file: on=%v err=%v, want off", on, err)
	}
	if err := os.WriteFile(tokenPath, []byte(`{"access_token":"a","refresh_token":"r","token_type":"Bearer"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if ts, on, err := TokenSource(cfg); !on || err != nil || ts == nil {
		t.Fatalf("configured: on=%v err=%v, want on", on, err)
	}
	if err := os.WriteFile(tokenPath, []byte(`not json`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, on, err := TokenSource(cfg); on || err == nil {
		t.Fatalf("broken token file: on=%v err=%v, want an error", on, err)
	}
}
