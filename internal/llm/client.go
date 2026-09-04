package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

const Model = "ministral-14b-2512"

const apiURL = "https://api.mistral.ai/v1/chat/completions"

const requestTimeout = 60 * time.Second

const maxRetries = 4
const baseBackoff = 2 * time.Second
const maxBackoff = 30 * time.Second

func isRetryableStatus(code int) bool {
	return code >= 500 && code < 600
}

func retryDelay(attempt int, retryAfterHeader string) time.Duration {
	if retryAfterHeader != "" {
		if secs, err := strconv.Atoi(retryAfterHeader); err == nil && secs > 0 {
			d := time.Duration(secs) * time.Second
			if d > maxBackoff {
				return maxBackoff
			}
			return d
		}
	}

	d := baseBackoff * time.Duration(1<<attempt)
	if d > maxBackoff {
		d = maxBackoff
	}
	jitter := time.Duration(rand.Int63n(int64(d) / 2))
	return d + jitter
}

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	ResponseFormat *responseFmt  `json:"response_format,omitempty"`
	Temperature    float64       `json:"temperature"`
}

type responseFmt struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) CompleteJSON(ctx context.Context, systemPrompt, userPrompt string, temperature float64) (string, error) {
	reqBody := chatRequest{
		Model: Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ResponseFormat: &responseFmt{Type: "json_object"},
		Temperature:    temperature,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	var lastErr error
	var retryAfterHeader string
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(retryDelay(attempt-1, retryAfterHeader)):
			}
		}

		result, nextRetryAfter, retryable, err := c.doRequest(ctx, bodyBytes)
		if err == nil {
			return result, nil
		}

		lastErr = err
		retryAfterHeader = nextRetryAfter

		if !retryable || attempt == maxRetries {
			return "", lastErr
		}
	}

	return "", lastErr
}

func (c *Client) doRequest(ctx context.Context, bodyBytes []byte) (result string, retryAfterHeader string, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", "", false, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", true, fmt.Errorf("calling mistral api: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", true, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", "", false, fmt.Errorf("mistral api returned status %d: %s", resp.StatusCode, truncate(string(respBytes), 500))
	}

	if isRetryableStatus(resp.StatusCode) {
		return "", resp.Header.Get("Retry-After"), true, fmt.Errorf("mistral api returned status %d: %s", resp.StatusCode, truncate(string(respBytes), 500))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", "", false, fmt.Errorf("parsing mistral response: %w (raw: %s)", err, truncate(string(respBytes), 500))
	}

	if parsed.Error != nil {
		return "", "", false, fmt.Errorf("mistral api error: %s", parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", false, fmt.Errorf("mistral api returned status %d: %s", resp.StatusCode, truncate(string(respBytes), 500))
	}
	if len(parsed.Choices) == 0 {
		return "", "", false, fmt.Errorf("mistral api returned no choices")
	}

	return parsed.Choices[0].Message.Content, "", false, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
