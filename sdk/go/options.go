package weknora

import (
	"errors"
	"net/http"
	"time"
)

// RetryPolicy controls how the SDK handles transient transport errors and
// 5xx responses. The defaults target three attempts with exponential
// backoff capped at 2s.
type RetryPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	RetryStatuses  []int
}

// ShouldRetry returns true when a transport error should be retried.
func (r RetryPolicy) ShouldRetry(attempt int, err error) bool {
	if r.MaxAttempts <= 0 || attempt+1 >= r.MaxAttempts {
		return false
	}
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrForbidden) {
		return false
	}
	return true
}

// ShouldRetryStatus returns true when an API status code should be retried.
func (r RetryPolicy) ShouldRetryStatus(attempt int, status int) bool {
	if r.MaxAttempts <= 0 || attempt+1 >= r.MaxAttempts {
		return false
	}
	for _, s := range r.RetryStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// Backoff returns the wait time before attempt N.
func (r RetryPolicy) Backoff(attempt int) time.Duration {
	delay := r.InitialBackoff
	if delay <= 0 {
		delay = 200 * time.Millisecond
	}
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay > r.MaxBackoff && r.MaxBackoff > 0 {
			delay = r.MaxBackoff
			break
		}
	}
	return delay
}

func defaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: 200 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
		RetryStatuses:  []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout},
	}
}

// options is the internal accumulator for Option setters.
type options struct {
	BaseURL     string
	HTTPClient  *http.Client
	Auth        Authenticator
	RetryPolicy RetryPolicy
}

func defaultOptions() options {
	return options{RetryPolicy: defaultRetryPolicy()}
}

// Option is a single configuration setter for NewClient.
type Option func(*options) error

// WithBaseURL sets the WeKnora API base URL (no trailing slash).
func WithBaseURL(u string) Option {
	return func(o *options) error {
		o.BaseURL = u
		return nil
	}
}

// WithHTTPClient replaces the default *http.Client. Use this to inject a
// custom RoundTripper (instrumentation, mTLS, mock transport for tests).
func WithHTTPClient(h *http.Client) Option {
	return func(o *options) error {
		o.HTTPClient = h
		return nil
	}
}

// WithBearerToken configures JWT bearer authentication.
func WithBearerToken(token string) Option {
	return func(o *options) error {
		if token == "" {
			return errors.New("weknora: empty bearer token")
		}
		o.Auth = &bearerAuth{token: token}
		return nil
	}
}

// WithAPIKey configures X-API-Key authentication for service-to-service calls.
func WithAPIKey(key string) Option {
	return func(o *options) error {
		if key == "" {
			return errors.New("weknora: empty api key")
		}
		o.Auth = &apiKeyAuth{key: key}
		return nil
	}
}

// WithRetryPolicy overrides the default retry policy.
func WithRetryPolicy(p RetryPolicy) Option {
	return func(o *options) error {
		o.RetryPolicy = p
		return nil
	}
}
