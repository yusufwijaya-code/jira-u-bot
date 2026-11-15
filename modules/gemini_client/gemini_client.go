// Package gemini_client calls Google's Gemini generateContent with a JSON
// response schema (Tech Spec "Papan Tiket Revamp & Buat Card Jira", Phase 2).
// The API key travels in the x-goog-api-key header only — never in a URL
// or a log line.
package gemini_client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	ErrInvalidOutput = errors.New("gemini: invalid output")
	ErrNotConfigured = errors.New("gemini: api key not configured")
)

// QuotaError is a 429: per-minute (RetryAfter set) or the daily quota.
type QuotaError struct {
	Daily      bool
	RetryAfter time.Duration
}

func (e *QuotaError) Error() string {
	if e.Daily {
		return "gemini: daily quota exhausted"
	}
	return fmt.Sprintf("gemini: rate limited, retry after %s", e.RetryAfter)
}

type Client struct {
	apiKey, model, baseURL string
	http                   *http.Client
}

func New(apiKey, model string) *Client {
	return &Client{apiKey: strings.TrimSpace(apiKey), model: model, baseURL: "https://generativelanguage.googleapis.com", http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) WithBaseURL(u string) *Client { c.baseURL = strings.TrimRight(u, "/"); return c }

func (c *Client) Configured() bool { return c.apiKey != "" }

func (c *Client) GenerateJSON(ctx context.Context, system, user string, schema map[string]any, out any) error {
	if !c.Configured() {
		return ErrNotConfigured
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		text, err := c.call(ctx, system, user, schema)
		if err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(text), out); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return fmt.Errorf("%w: %v", ErrInvalidOutput, lastErr)
}

func (c *Client) call(ctx context.Context, system, user string, schema map[string]any) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"systemInstruction": map[string]any{"parts": []map[string]any{{"text": system}}},
		"contents":          []map[string]any{{"role": "user", "parts": []map[string]any{{"text": user}}}},
		"generationConfig": map[string]any{
			"temperature":      0,
			"responseMimeType": "application/json",
			"responseSchema":   schema,
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1beta/models/%s:generateContent", c.baseURL, c.model), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", classifyQuota(raw)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gemini: status %d", resp.StatusCode)
	}
	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("%w: empty candidates", ErrInvalidOutput)
	}
	return parsed.Candidates[0].Content.Parts[0].Text, nil
}

func classifyQuota(raw []byte) error {
	var body struct {
		Error struct {
			Details []struct {
				Type       string `json:"@type"`
				RetryDelay string `json:"retryDelay"`
				Violations []struct {
					QuotaID string `json:"quotaId"`
				} `json:"violations"`
			} `json:"details"`
		} `json:"error"`
	}
	q := &QuotaError{RetryAfter: time.Minute}
	if json.Unmarshal(raw, &body) != nil {
		return q
	}
	for _, d := range body.Error.Details {
		for _, v := range d.Violations {
			if strings.Contains(v.QuotaID, "PerDay") {
				q.Daily = true
			}
		}
		if d.RetryDelay != "" {
			if dur, err := time.ParseDuration(d.RetryDelay); err == nil {
				q.RetryAfter = dur
			}
		}
	}
	return q
}
