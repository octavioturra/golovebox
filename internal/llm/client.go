package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type Config struct {
	BaseURL     string
	APIKey      string
	Model       string
	MaxAttempts int           // default 3
	BaseDelay   time.Duration // default 2s
	MaxDelay    time.Duration // default 30s
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.BaseDelay == 0 {
		cfg.BaseDelay = 2 * time.Second
	}
	if cfg.MaxDelay == 0 {
		cfg.MaxDelay = 30 * time.Second
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 120 * time.Second},
	}
}

// statusError carries the HTTP status code so retry logic can inspect it.
type statusError struct {
	code int
	body string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("llm: status %d: %s", e.code, e.body)
}

// Complete sends messages to the LLM and returns the assistant reply.
// Retries on transient errors (429, 5xx, network) with exponential backoff.
func (c *Client) Complete(ctx context.Context, messages []Message) (string, error) {
	var lastErr error
	for attempt := 0; attempt < c.cfg.MaxAttempts; attempt++ {
		result, err := c.doComplete(ctx, messages)
		if err == nil {
			return result, nil
		}
		if !isRetryable(err) {
			return "", err
		}
		lastErr = err
		delay := retryDelay(c.cfg.BaseDelay, c.cfg.MaxDelay, attempt)
		fmt.Fprintf(os.Stderr, "llm: attempt %d/%d failed: %v, retrying in %v\n",
			attempt+1, c.cfg.MaxAttempts, err, delay)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
		}
	}
	return "", lastErr
}

func (c *Client) doComplete(ctx context.Context, messages []Message) (string, error) {
	type requestBody struct {
		Model     string    `json:"model"`
		MaxTokens int       `json:"max_tokens"`
		Messages  []Message `json:"messages"`
	}

	payload, err := json.Marshal(requestBody{
		Model:     c.cfg.Model,
		MaxTokens: 8192,
		Messages:  messages,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.BaseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	if strings.Contains(c.cfg.BaseURL, "anthropic") {
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llm: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", &statusError{code: resp.StatusCode, body: string(body)}
	}

	// Unified response struct — handles both Anthropic (content[0].text)
	// and OpenAI-compat (choices[0].message.content) without provider conditionals.
	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("llm: parse response: %w", err)
	}
	if len(parsed.Content) > 0 {
		return parsed.Content[0].Text, nil
	}
	if len(parsed.Choices) > 0 {
		return parsed.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("llm: empty response body: %s", string(body))
}

func isRetryable(err error) bool {
	var se *statusError
	if errors.As(err, &se) {
		switch se.code {
		case 429, 500, 502, 503, 504:
			return true
		default:
			return false
		}
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func retryDelay(base, maxDelay time.Duration, attempt int) time.Duration {
	delay := base * time.Duration(1<<uint(attempt))
	// Overflow / unreasonably large delay guard
	if delay <= 0 {
		delay = maxDelay
	}
	// ±20% jitter
	jitterRange := int64(delay / 5)
	if jitterRange > 0 {
		jitter := time.Duration(rand.Int63n(jitterRange*2+1)) - time.Duration(jitterRange)
		delay += jitter
	}
	if delay > maxDelay {
		delay = maxDelay
	}
	if delay < time.Millisecond {
		delay = time.Millisecond
	}
	return delay
}
