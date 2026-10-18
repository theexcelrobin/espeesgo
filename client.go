package espeesgo

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

const (
	Version = "0.2.0"

	DefaultAPIURL      = "https://api.espees.org"
	DefaultCheckoutURL = "https://payment.espees.org"

	defaultTimeout  = 30 * time.Second
	maxResponseSize = 10 << 20
)

// Client talks to the Espees API. It is safe for concurrent use.
type Client struct {
	apiKey      string
	apiURL      string
	checkoutURL string
	userAgent   string
	timeout     time.Duration
	maxRetries  int
	version     APIVersion
	http        *http.Client
}

// New creates a Client.
func New(opts ...Option) *Client {
	c := &Client{
		apiURL:      DefaultAPIURL,
		checkoutURL: DefaultCheckoutURL,
		userAgent:   "espeesgo/" + Version,
		timeout:     defaultTimeout,
		version:     APIv1,
		http:        &http.Client{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	return c.call(ctx, path, body, out, true)
}

func (c *Client) call(ctx context.Context, path string, body, out any, keyRequired bool) error {
	if keyRequired && c.apiKey == "" {
		return ErrNoAPIKey
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	for attempt := 0; ; attempt++ {
		err := c.send(ctx, path, payload, out)

		var apiErr *APIError
		if err == nil || attempt >= c.maxRetries || !errors.As(err, &apiErr) || apiErr.Status != http.StatusTooManyRequests {
			return err
		}

		wait := apiErr.RetryAfter
		if wait <= 0 {
			wait = time.Second
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

func (c *Client) send(ctx context.Context, path string, payload []byte, out any) error {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return newAPIError(resp.StatusCode, resp.Header, data)
	}

	var env envelope
	if json.Unmarshal(data, &env) == nil {
		if env.ErrorMessage != "" {
			return &APIError{Status: http.StatusBadGateway, Message: strings.TrimSpace(env.ErrorType + ": " + env.ErrorMessage)}
		}
		if status, ok := env.status(); ok && (status < 200 || status > 299) {
			return &APIError{Status: status, Message: env.text()}
		}
	}

	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("espeesgo: decode response: %w", err)
	}
	return nil
}
