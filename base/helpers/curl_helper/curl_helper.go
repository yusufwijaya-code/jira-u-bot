package curl_helper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// CurlRequest is the one generic HTTP client used for every outbound call in
// this service (Jira Cloud API, Google Chat webhook) — mirrors the shape of
// go-core-api's base/helpers/curl_helper, re-implemented locally (spec Note C1).
func CurlRequest[T any](ctx context.Context, url string, headers map[string]string, body any, method string, timeoutMs int, returnJSON bool) (*T, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("curl_helper: failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("curl_helper: failed to build request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	timeout := 10 * time.Second
	if timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}
	client := &http.Client{Timeout: timeout}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("curl_helper: request to %s failed: %w", redactURL(url), err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("curl_helper: failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	if !returnJSON {
		return nil, nil
	}

	var result T
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("curl_helper: failed to unmarshal response: %w", err)
		}
	}
	return &result, nil
}

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("curl_helper: unexpected status %d: %s", e.StatusCode, e.Body)
}

// redactURL strips userinfo and the query string before a URL is ever
// embedded in an error message — several callers (e.g. the Google Chat
// webhook) carry secrets in the query string, and errors routinely end up
// in logs.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[unparseable URL redacted]"
	}
	u.User = nil
	u.RawQuery = ""
	return u.String()
}
