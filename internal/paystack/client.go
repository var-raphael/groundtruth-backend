package paystack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://api.paystack.co"

type Client struct {
	secret string
	http   *http.Client
}

type Plan struct {
	PlanCode string `json:"plan_code"`
	Name     string `json:"name"`
	Amount   int    `json:"amount"`
	Currency string `json:"currency"`
	Interval string `json:"interval"`
}

func NewClient(secret string) *Client {
	return &Client{secret: secret, http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) Configured() bool {
	return c != nil && c.secret != ""
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding paystack request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("building paystack request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("calling paystack: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("reading paystack response: %w", err)
	}

	var envelope struct {
		Status  bool            `json:"status"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("paystack returned status %d with an unreadable body", resp.StatusCode)
	}
	if resp.StatusCode >= 400 || !envelope.Status {
		return fmt.Errorf("paystack: %s", envelope.Message)
	}
	if out != nil && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("decoding paystack data: %w", err)
		}
	}
	return nil
}

func (c *Client) FetchPlan(ctx context.Context, code string) (*Plan, error) {
	var plan Plan
	if err := c.do(ctx, http.MethodGet, "/plan/"+code, nil, &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

func (c *Client) InitializeTransaction(ctx context.Context, email string, amount int, plan, callbackURL string, metadata map[string]string) (string, error) {
	var data struct {
		AuthorizationURL string `json:"authorization_url"`
	}
	err := c.do(ctx, http.MethodPost, "/transaction/initialize", map[string]any{
		"email":        email,
		"amount":       amount,
		"plan":         plan,
		"callback_url": callbackURL,
		"metadata":     metadata,
		"channels":     []string{"card"},
	}, &data)
	if err != nil {
		return "", err
	}
	if data.AuthorizationURL == "" {
		return "", fmt.Errorf("paystack did not return a checkout link")
	}
	return data.AuthorizationURL, nil
}

func (c *Client) DisableSubscription(ctx context.Context, code, token string) error {
	return c.do(ctx, http.MethodPost, "/subscription/disable", map[string]string{"code": code, "token": token}, nil)
}

func (c *Client) VerifySignature(body []byte, signature string) bool {
	if c.secret == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha512.New, []byte(c.secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
