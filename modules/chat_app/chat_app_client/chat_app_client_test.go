package chat_app_client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"

	"nudge/modules/chat_app/chat_app_dto"
)

func serve(t *testing.T, status int, check func(r *http.Request, body map[string]any), reply string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		check(r, body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	old := chatAPIBase
	chatAPIBase = srv.URL
	t.Cleanup(func() { chatAPIBase = old })
}

func TestCreateMessage_PostsWithServiceAccountToken(t *testing.T) {
	serve(t, http.StatusOK, func(r *http.Request, body map[string]any) {
		if r.Method != http.MethodPost || r.URL.Path != "/spaces/AAA/messages" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sa-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if body["text"] != "halo" || body["markupSyntax"] != "MARKUP_SYNTAX_CHAT" {
			t.Errorf("body = %v", body)
		}
	}, `{}`)
	c := New(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "sa-token"}))
	if err := c.CreateMessage(context.Background(), "spaces/AAA", chat_app_dto.OutgoingMessage{Text: "halo"}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
}

func TestCreateMessage_GoneOnlyFor403And404(t *testing.T) {
	for status, gone := range map[int]bool{404: true, 403: true, 500: false, 400: false} {
		serve(t, status, func(*http.Request, map[string]any) {}, `{"error":{}}`)
		c := New(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "t"}))
		err := c.CreateMessage(context.Background(), "spaces/AAA", chat_app_dto.OutgoingMessage{Text: "x"})
		if err == nil {
			t.Fatalf("status %d: want error", status)
		}
		if IsGone(err) != gone {
			t.Errorf("status %d: IsGone = %v, want %v", status, IsGone(err), gone)
		}
	}
	if IsGone(errors.New("network")) {
		t.Error("IsGone(plain error) = true")
	}
}

func TestSetupBotDM_UsesUserTokenAndSingleUserBotDm(t *testing.T) {
	serve(t, http.StatusOK, func(r *http.Request, body map[string]any) {
		if r.URL.Path != "/spaces:setup" || r.Header.Get("Authorization") != "Bearer user-token" {
			t.Errorf("request = %s auth %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		space, _ := body["space"].(map[string]any)
		if space["spaceType"] != "DIRECT_MESSAGE" || space["singleUserBotDm"] != true {
			t.Errorf("body = %v", body)
		}
	}, `{"name":"spaces/DM1"}`)
	name, err := SetupBotDM(context.Background(), "user-token")
	if err != nil || name != "spaces/DM1" {
		t.Fatalf("SetupBotDM = %q, %v", name, err)
	}
}

func TestNewFromFile_MissingFile(t *testing.T) {
	if _, err := NewFromFile("does-not-exist.json"); err == nil {
		t.Error("want an error for a missing key file")
	}
}
