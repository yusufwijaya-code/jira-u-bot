package curl_helper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type testResponse struct {
	Message string `json:"message"`
}

func TestCurlRequest_GETSuccessDecodesJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(testResponse{Message: "ok"})
	}))
	defer server.Close()

	result, err := CurlRequest[testResponse](context.Background(), server.URL, nil, nil, http.MethodGet, 1000, true)
	if err != nil {
		t.Fatalf("CurlRequest returned error: %v", err)
	}
	if result.Message != "ok" {
		t.Errorf("Message = %q, want %q", result.Message, "ok")
	}
}

func TestCurlRequest_SendsHeadersAndBody(t *testing.T) {
	type reqBody struct {
		Name string `json:"name"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization header = %q, want %q", got, "Bearer secret")
		}
		var body reqBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body.Name != "jira-u-bot" {
			t.Errorf("body.Name = %q, want %q", body.Name, "jira-u-bot")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(testResponse{Message: "received"})
	}))
	defer server.Close()

	headers := map[string]string{"Authorization": "Bearer secret"}
	result, err := CurlRequest[testResponse](context.Background(), server.URL, headers, reqBody{Name: "jira-u-bot"}, http.MethodPost, 1000, true)
	if err != nil {
		t.Fatalf("CurlRequest returned error: %v", err)
	}
	if result.Message != "received" {
		t.Errorf("Message = %q, want %q", result.Message, "received")
	}
}

func TestCurlRequest_NonSuccessStatusReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()

	_, err := CurlRequest[testResponse](context.Background(), server.URL, nil, nil, http.MethodGet, 1000, true)
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("error type = %T, want *HTTPError", err)
	}
	if httpErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", httpErr.StatusCode, http.StatusNotFound)
	}
}

func TestCurlRequest_TimeoutReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := CurlRequest[testResponse](context.Background(), server.URL, nil, nil, http.MethodGet, 10, true)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestRedactURL_StripsQueryStringAndUserinfo(t *testing.T) {
	got := redactURL("https://user:pass@chat.googleapis.com/v1/spaces/x/messages?key=secret&token=secret2")
	want := "https://chat.googleapis.com/v1/spaces/x/messages"
	if got != want {
		t.Errorf("redactURL = %q, want %q", got, want)
	}
}

func TestCurlRequest_ReturnJSONFalseSkipsDecoding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid json{{{"))
	}))
	defer server.Close()

	result, err := CurlRequest[testResponse](context.Background(), server.URL, nil, nil, http.MethodGet, 1000, false)
	if err != nil {
		t.Fatalf("CurlRequest returned error: %v", err)
	}
	if result != nil {
		t.Errorf("result = %+v, want nil when returnJSON is false", result)
	}
}
