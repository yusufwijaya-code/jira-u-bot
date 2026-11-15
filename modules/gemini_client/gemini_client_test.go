package gemini_client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type out struct {
	Rows []struct {
		Title string `json:"title"`
	} `json:"rows"`
}

func reply(w http.ResponseWriter, text string) {
	json.NewEncoder(w).Encode(map[string]any{"candidates": []map[string]any{{"content": map[string]any{"parts": []map[string]any{{"text": text}}}}}})
}

func TestGenerateJSON_SendsSchemaAndKeyInHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-flash-lite-latest:generateContent" || r.URL.RawQuery != "" {
			t.Errorf("url = %s?%s (key must not be in the URL)", r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("x-goog-api-key") != "k" {
			t.Errorf("missing api key header")
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gc := body["generationConfig"].(map[string]any)
		if gc["temperature"].(float64) != 0 || gc["responseMimeType"] != "application/json" || gc["responseSchema"] == nil {
			t.Errorf("generationConfig = %v", gc)
		}
		reply(w, `{"rows":[{"title":"A"}]}`)
	}))
	defer srv.Close()
	var o out
	err := New("k", "gemini-flash-lite-latest").WithBaseURL(srv.URL).GenerateJSON(context.Background(), "sys", "user", map[string]any{"type": "OBJECT"}, &o)
	if err != nil || len(o.Rows) != 1 || o.Rows[0].Title != "A" {
		t.Fatalf("o = %+v, err = %v", o, err)
	}
}

func TestGenerateJSON_RetriesInvalidJSONOnce(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		reply(w, "not json")
	}))
	defer srv.Close()
	var o out
	err := New("k", "m").WithBaseURL(srv.URL).GenerateJSON(context.Background(), "s", "u", map[string]any{}, &o)
	if !errors.Is(err, ErrInvalidOutput) || calls != 2 {
		t.Fatalf("err = %v, calls = %d; want ErrInvalidOutput after 2 calls", err, calls)
	}
}

func TestGenerateJSON_ClassifiesQuota(t *testing.T) {
	for _, tc := range []struct {
		body  string
		daily bool
	}{
		{`{"error":{"code":429,"message":"Quota exceeded","details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier"}]}]}}`, true},
		{`{"error":{"code":429,"message":"Quota exceeded","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"42s"}]}}`, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(tc.body))
		}))
		var q *QuotaError
		err := New("k", "m").WithBaseURL(srv.URL).GenerateJSON(context.Background(), "s", "u", map[string]any{}, &out{})
		srv.Close()
		if !errors.As(err, &q) || q.Daily != tc.daily {
			t.Fatalf("body %s: err = %v, want QuotaError{Daily:%v}", tc.body, err, tc.daily)
		}
		if !tc.daily && q.RetryAfter != 42*time.Second {
			t.Errorf("RetryAfter = %v, want 42s", q.RetryAfter)
		}
	}
}

func TestGenerateJSON_NotConfigured(t *testing.T) {
	err := New("", "m").GenerateJSON(context.Background(), "s", "u", map[string]any{}, &out{})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
	if New(" ", "m").Configured() {
		t.Fatal("blank key must not count as configured")
	}
}
