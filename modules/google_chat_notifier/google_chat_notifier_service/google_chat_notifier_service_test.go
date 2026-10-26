package google_chat_notifier_service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jira-u-bot/config"
	"jira-u-bot/modules/google_chat_notifier/google_chat_notifier_dto"
)

func TestSendReminder_PostsExpectedTextToWebhook(t *testing.T) {
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	cfg := &config.AppConfig{GoogleChatWebhookURL: server.URL}
	svc := NewGoogleChatNotifierService(cfg)

	msg := google_chat_notifier_dto.ReminderMessage{
		IssueKey:       "OMS-1",
		IssueURL:       "https://esb-id.atlassian.net/browse/OMS-1",
		IssueSummary:   "Book Integration",
		AssigneeName:   "Anwar",
		StoryPoints:    3,
		BudgetSeconds:  10800,
		ElapsedSeconds: 9000,
		Tier:           "30m",
		IsOverage:      false,
	}

	if err := svc.SendReminder(context.Background(), msg); err != nil {
		t.Fatalf("SendReminder returned error: %v", err)
	}

	text := gotBody["text"]
	if !strings.Contains(text, "OMS-1") {
		t.Errorf("message text %q does not contain issue key", text)
	}
	if !strings.Contains(text, "Anwar") {
		t.Errorf("message text %q does not contain assignee name", text)
	}
	if !strings.Contains(text, "Book Integration") {
		t.Errorf("message text %q does not contain issue summary", text)
	}
}

func TestSendReminder_OverageMessageFlagsExceededBudget(t *testing.T) {
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	cfg := &config.AppConfig{GoogleChatWebhookURL: server.URL}
	svc := NewGoogleChatNotifierService(cfg)

	msg := google_chat_notifier_dto.ReminderMessage{
		IssueKey:       "OMS-1",
		IssueURL:       "https://esb-id.atlassian.net/browse/OMS-1",
		IssueSummary:   "Book Integration",
		AssigneeName:   "Anwar",
		StoryPoints:    3,
		BudgetSeconds:  10800,
		ElapsedSeconds: 12000,
		Tier:           "overage",
		IsOverage:      true,
	}

	if err := svc.SendReminder(context.Background(), msg); err != nil {
		t.Fatalf("SendReminder returned error: %v", err)
	}

	text := gotBody["text"]
	if !strings.Contains(text, "LEWAT BUDGET") {
		t.Errorf("overage message text %q should flag that budget is exceeded", text)
	}
}

func TestSendReminder_TierMessageShowsDeadlineAndDHMSBreakdown(t *testing.T) {
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	cfg := &config.AppConfig{GoogleChatWebhookURL: server.URL}
	svc := NewGoogleChatNotifierService(cfg)

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("failed to load Asia/Jakarta location: %v", err)
	}
	deadline := time.Date(2026, 10, 26, 15, 12, 0, 0, loc) // Monday

	msg := google_chat_notifier_dto.ReminderMessage{
		IssueKey:       "OMS-12461",
		IssueURL:       "https://esb-id.atlassian.net/browse/OMS-12461",
		IssueSummary:   "API Create Menu Recommendation",
		AssigneeName:   "Yusuf Wijaya",
		StoryPoints:    4,
		BudgetSeconds:  14400, // 4h
		ElapsedSeconds: 11160, // 3.1h -> remaining 3240s = 0h 54m 0s
		Tier:           "2h",
		IsOverage:      false,
		Deadline:       deadline,
	}

	if err := svc.SendReminder(context.Background(), msg); err != nil {
		t.Fatalf("SendReminder returned error: %v", err)
	}

	text := gotBody["text"]
	if !strings.Contains(text, "Harus selesai: Senin, 26 Okt 2026 15:12:00 WIB") {
		t.Errorf("message text %q does not contain the expected absolute deadline line", text)
	}
	if !strings.Contains(text, "Sisa: 0 hari 0 jam 54 menit 0 detik") {
		t.Errorf("message text %q does not contain the expected D/H/M/S remaining breakdown", text)
	}
}

func TestSendReminder_OverageMessageOmitsDeadlineButShowsDHMSBreakdown(t *testing.T) {
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	cfg := &config.AppConfig{GoogleChatWebhookURL: server.URL}
	svc := NewGoogleChatNotifierService(cfg)

	msg := google_chat_notifier_dto.ReminderMessage{
		IssueKey:       "OMS-1",
		IssueURL:       "https://esb-id.atlassian.net/browse/OMS-1",
		IssueSummary:   "Book Integration",
		AssigneeName:   "Anwar",
		StoryPoints:    3,
		BudgetSeconds:  10800,
		ElapsedSeconds: 15660, // overage by 4860s = 1h 21m 0s
		Tier:           "overage",
		IsOverage:      true,
	}

	if err := svc.SendReminder(context.Background(), msg); err != nil {
		t.Fatalf("SendReminder returned error: %v", err)
	}

	text := gotBody["text"]
	if strings.Contains(text, "Harus selesai:") {
		t.Errorf("overage message text %q must not show an absolute deadline — it has already passed", text)
	}
	if !strings.Contains(text, "Overage: 0 hari 1 jam 21 menit 0 detik") {
		t.Errorf("message text %q does not contain the expected D/H/M/S overage breakdown", text)
	}
}

func TestSendReminder_SkipsWhenWebhookURLUnset(t *testing.T) {
	cfg := &config.AppConfig{GoogleChatWebhookURL: ""}
	svc := NewGoogleChatNotifierService(cfg)

	err := svc.SendReminder(context.Background(), google_chat_notifier_dto.ReminderMessage{IssueKey: "OMS-1"})
	if err != nil {
		t.Fatalf("SendReminder returned error: %v, want nil (no-op when webhook unset, mirrors go-core-api's notifier)", err)
	}
}

func TestSendReminder_PropagatesErrorOnNon2xxResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := &config.AppConfig{GoogleChatWebhookURL: server.URL}
	svc := NewGoogleChatNotifierService(cfg)

	err := svc.SendReminder(context.Background(), google_chat_notifier_dto.ReminderMessage{IssueKey: "OMS-1"})
	if err == nil {
		t.Fatal("expected error when webhook returns 500 — spec Reliability requires callers to NOT write reminder_log when this errors")
	}
}
