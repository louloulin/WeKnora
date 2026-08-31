package weknora

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

)

// Client is the top-level WeKnora SDK entry point. It holds the configured
// HTTP transport, base URL, and auth material, and exposes one sub-service
// per WeKnora REST surface (knowledge bases, search, chat, ...).
type Client struct {
	baseURL     string
	http        *http.Client
	auth        Authenticator
	retryPolicy RetryPolicy

	// Sub-services are constructed once and reused.
	KnowledgeBase    *KnowledgeBaseService
	Search           *SearchService
	Chat             *ChatService
	Conversation     *ConversationService
	Database         *DatabaseService
	Formula          *FormulaService
	Automation       *AutomationService
	CollabDoc        *CollabDocService
	AgentStudio      *AgentStudioService
	Connector        *ConnectorService
	Verification     *VerificationService
}

// NewClient constructs a Client from the provided options. The base URL and
// one of bearer / API-key authentication are required; everything else is
// optional and falls back to safe defaults.
func NewClient(ctx context.Context, opts ...Option) (*Client, error) {
	o := defaultOptions()
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return nil, err
		}
	}
	if o.BaseURL == "" {
		return nil, fmt.Errorf("weknora: BaseURL is required")
	}
	if o.Auth == nil {
		return nil, fmt.Errorf("weknora: authenticator is required (use WithBearerToken or WithAPIKey)")
	}

	httpClient := o.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	c := &Client{
		baseURL:     strings.TrimRight(o.BaseURL, "/"),
		http:        httpClient,
		auth:        o.Auth,
		retryPolicy: o.RetryPolicy,
	}
	c.KnowledgeBase = NewKnowledgeBaseService(c)
	c.Search = NewSearchService(c)
	c.Chat = NewChatService(c)
	c.Conversation = NewConversationService(c)
	c.Database = NewDatabaseService(c)
	c.Formula = NewFormulaService(c)
	c.Automation = NewAutomationService(c)
	c.CollabDoc = NewCollabDocService(c)
	c.AgentStudio = NewAgentStudioService(c)
	c.Connector = NewConnectorService(c)
	c.Verification = NewVerificationService(c)
	return c, nil
}

// BaseURL exposes the normalized base URL the client uses. Useful when the
// caller wants to share a base URL with other tools.
func (c *Client) BaseURL() string { return c.baseURL }

// do executes a single request with the configured auth, retry policy, and
// JSON encoding/decoding. Service-specific helpers in services/* are thin
// wrappers that call this.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	full := c.baseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("weknora: marshal request: %w", err)
		}
		bodyReader = strings.NewReader(string(buf))
	}
	req, err := http.NewRequestWithContext(ctx, method, full, bodyReader)
	if err != nil {
		return fmt.Errorf("weknora: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.auth.Apply(req); err != nil {
		return fmt.Errorf("weknora: apply auth: %w", err)
	}

	var lastErr error
	attempts := c.retryPolicy.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			if !c.retryPolicy.ShouldRetry(attempt, err) {
				return fmt.Errorf("weknora: http: %w", err)
			}
			time.Sleep(c.retryPolicy.Backoff(attempt))
			continue
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if out == nil {
					return
				}
				if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
					lastErr = fmt.Errorf("%w: decode body: %v", ErrInvalidResponse, err)
				}
				return
			}
			var apiErr struct {
				Error string `json:"error"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&apiErr)
			api := &APIError{
				StatusCode: resp.StatusCode,
				Code:       http.StatusText(resp.StatusCode),
				Message:    apiErr.Error,
			}
			lastErr = api
		}()
		if lastErr == nil {
			return nil
		}
		if api, ok := lastErr.(*APIError); ok {
			if !c.retryPolicy.ShouldRetryStatus(attempt, api.StatusCode) {
				return api
			}
		}
		time.Sleep(c.retryPolicy.Backoff(attempt))
	}
	if lastErr == nil {
		return ErrServer
	}
	return lastErr
}
