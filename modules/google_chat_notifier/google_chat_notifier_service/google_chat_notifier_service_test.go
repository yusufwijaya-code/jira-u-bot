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
	if !strings.Contains(text, "*Harus Selesai Pass:* Senin, 26 Okt 2026 15:12:00 WIB") {
		t.Errorf("message text %q does not contain the expected bold absolute deadline line", text)
	}
	if !strings.Contains(text, "*Sisa waktu:* 0 hari 0 jam 54 menit 0 detik") {
		t.Errorf("message text %q does not contain the expected bold D/H/M/S remaining breakdown", text)
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
	if strings.Contains(text, "Harus Selesai Pass:") {
		t.Errorf("overage message text %q must not show an absolute deadline — it has already passed", text)
	}
	if !strings.Contains(text, "*Lewat budget selama:* 0 hari 1 jam 21 menit 0 detik") {
		t.Errorf("message text %q does not contain the expected bold D/H/M/S overage breakdown", text)
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

func TestSendReminder_EarlyCompletionMessageShowsUnusedBudgetAndWouldHaveBeenDeadline(t *testing.T) {
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
	deadline := time.Date(2026, 10, 27, 9, 0, 0, 0, loc) // Tuesday

	msg := google_chat_notifier_dto.ReminderMessage{
		IssueKey:          "OMS-1",
		IssueURL:          "https://esb-id.atlassian.net/browse/OMS-1",
		IssueSummary:      "Early finisher",
		AssigneeName:      "Teammate",
		StoryPoints:       3,
		BudgetSeconds:     10800, // 3h
		ElapsedSeconds:    5400,  // 1.5h -> remaining 5400s = 1h 30m
		IsEarlyCompletion: true,
		Deadline:          deadline,
	}

	if err := svc.SendReminder(context.Background(), msg); err != nil {
		t.Fatalf("SendReminder returned error: %v", err)
	}

	text := gotBody["text"]
	if !strings.Contains(text, "Selesai Lebih Awal") {
		t.Errorf("message text %q does not contain the expected early-completion header", text)
	}
	if !strings.Contains(text, "*Sisa budget tidak terpakai:* 0 hari 1 jam 30 menit 0 detik") {
		t.Errorf("message text %q does not contain the expected unused-budget line", text)
	}
	if !strings.Contains(text, "*Limit seharusnya selesai:* Selasa, 27 Okt 2026 09:00:00 WIB") {
		t.Errorf("message text %q does not contain the expected would-have-been-deadline line", text)
	}
	if strings.Contains(text, "Harus Selesai Pass:") {
		t.Error("early-completion message must not show the tier-reminder 'Harus Selesai Pass' line")
	}
}

func TestBuildMessageText_UsesPlainURLNotMaskedHyperlink(t *testing.T) {
	text := BuildMessageText(google_chat_notifier_dto.ReminderMessage{
		IssueKey:     "OMS-1",
		IssueURL:     "https://esb-id.atlassian.net/browse/OMS-1",
		IssueSummary: "Book Integration",
		AssigneeName: "Anwar",
		Tier:         "30m",
	})

	if !strings.Contains(text, "https://esb-id.atlassian.net/browse/OMS-1") {
		t.Errorf("message text %q does not contain the plain issue URL", text)
	}
	if strings.Contains(text, "<https://esb-id.atlassian.net/browse/OMS-1|") {
		t.Error("message text must not use the <url|label> masked-hyperlink syntax — it renders as literal text instead of a link when sent via the DM (user-OAuth) path, unlike the webhook path")
	}
}

func TestBuildMessageText_IncludesBoldCodeProcessStartedAt(t *testing.T) {
	startedAt := time.Date(2026, 10, 27, 9, 50, 47, 0, time.FixedZone("WIB", 7*3600))
	text := BuildMessageText(google_chat_notifier_dto.ReminderMessage{
		IssueKey:             "OMS-1",
		AssigneeName:         "Anwar",
		Tier:                 "30m",
		CodeProcessStartedAt: startedAt,
	})

	if !strings.Contains(text, "*Masuk Code Process:* Selasa, 27 Okt 2026 09:50:47 WIB") {
		t.Errorf("message text %q does not contain the expected bold Code Process start line", text)
	}
}

func TestBuildMessageText_EarlyCompletionIncludesCodeReviewEnteredAt(t *testing.T) {
	enteredAt := time.Date(2026, 10, 27, 11, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	text := BuildMessageText(google_chat_notifier_dto.ReminderMessage{
		IssueKey:            "OMS-1",
		AssigneeName:        "Anwar",
		IsEarlyCompletion:   true,
		CodeReviewEnteredAt: enteredAt,
	})

	if !strings.Contains(text, "*Pindah ke Code Review:* Selasa, 27 Okt 2026 11:00:00 WIB") {
		t.Errorf("message text %q does not contain the expected bold Code Review transition line", text)
	}
}

func TestBuildMessageText_TierAndOverageOmitCodeReviewEnteredAt(t *testing.T) {
	for _, msg := range []google_chat_notifier_dto.ReminderMessage{
		{IssueKey: "OMS-1", Tier: "30m"},
		{IssueKey: "OMS-1", IsOverage: true},
	} {
		if text := BuildMessageText(msg); strings.Contains(text, "Pindah ke Code Review") {
			t.Errorf("message text %q must not show 'Pindah ke Code Review' — ticket hasn't left Code Process yet", text)
		}
	}
}

func TestBuildMessageText_IncludesStoryPointCalculationExplanation(t *testing.T) {
	text := BuildMessageText(google_chat_notifier_dto.ReminderMessage{
		IssueKey:      "OMS-1",
		AssigneeName:  "Anwar",
		Tier:          "30m",
		StoryPoints:   3,
		BudgetSeconds: 10800,
	})

	if !strings.Contains(text, "*Story Point:* 3.0 SP (budget 3.0 jam, dihitung dari 3.0 SP × 1 jam/SP)") {
		t.Errorf("message text %q does not explain how the budget was calculated from story points", text)
	}
}

func TestBuildMessageText_EndsWithOKRHashtagFooter(t *testing.T) {
	for _, msg := range []google_chat_notifier_dto.ReminderMessage{
		{IssueKey: "OMS-1", Tier: "30m"},
		{IssueKey: "OMS-1", IsOverage: true},
		{IssueKey: "OMS-1", IsEarlyCompletion: true},
	} {
		text := BuildMessageText(msg)
		if !strings.HasSuffix(text, "#BersamaSayaMenempuhOKRyangPassss") {
			t.Errorf("message text %q does not end with the OKR hashtag footer", text)
		}
	}
}

func TestBuildMessageText_IncludesGeserHintForTierAndOverageOnly(t *testing.T) {
	cases := []struct {
		name     string
		msg      google_chat_notifier_dto.ReminderMessage
		wantHint bool
	}{
		{"tier reminder", google_chat_notifier_dto.ReminderMessage{IssueKey: "OMS-1", Tier: "30m"}, true},
		{"overage", google_chat_notifier_dto.ReminderMessage{IssueKey: "OMS-1", IsOverage: true}, true},
		{"early completion", google_chat_notifier_dto.ReminderMessage{IssueKey: "OMS-1", IsEarlyCompletion: true}, false},
	}
	for _, c := range cases {
		text := BuildMessageText(c.msg)
		hasHint := strings.Contains(text, "/u-geser<OMS-1>:todo") && strings.Contains(text, "/u-geser<OMS-1>:code-review")
		if hasHint != c.wantHint {
			t.Errorf("%s: geser hint present = %v, want %v — text = %q", c.name, hasHint, c.wantHint, text)
		}
	}
}

func TestBuildMessageText_IncludesHelpHint(t *testing.T) {
	for _, msg := range []google_chat_notifier_dto.ReminderMessage{
		{IssueKey: "OMS-1", Tier: "30m"},
		{IssueKey: "OMS-1", IsOverage: true},
		{IssueKey: "OMS-1", IsEarlyCompletion: true},
	} {
		text := BuildMessageText(msg)
		if !strings.Contains(text, "/u-help") {
			t.Errorf("message text %q does not mention /u-help — every reminder/overage/early-completion message (space and DM alike, since both reuse this function) must advertise the interactive commands", text)
		}
	}
}
