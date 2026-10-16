package espeesgo

import (
	"net/http"
	"strings"
	"time"
)

// Option configures a Client.
type Option func(*Client)

// WithAPIKey sets the API key sent in the x-api-key header. Keep it on your
// server and never ship it to a browser or mobile app.
func WithAPIKey(key string) Option {
	return func(c *Client) { c.apiKey = key }
}

// WithHTTPClient replaces the default http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.http = hc
		}
	}
}

// WithAPIURL overrides the API host, which is useful for tests.
func WithAPIURL(u string) Option {
	return func(c *Client) { c.apiURL = strings.TrimRight(u, "/") }
}

// WithCheckoutURL overrides the host buyers are sent to for payment.
func WithCheckoutURL(u string) Option {
	return func(c *Client) { c.checkoutURL = strings.TrimRight(u, "/") }
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// WithTimeout sets the per-request timeout. Zero disables it.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// WithMaxRetries retries a request up to n more times after a 429. Other
// failures are never retried, as a vend must not be sent twice.
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		if n > 0 {
			c.maxRetries = n
		}
	}
}
