package weknora

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Do executes an HTTP request with the standard JSON request / response
// envelope. The path is appended to the client's base URL verbatim; callers
// are responsible for including any leading slash.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	return c.do(ctx, method, path, query, body, out)
}

// DoQuery is a convenience wrapper that accepts a map[string][]string for
// the query string and encodes it for the caller.
func (c *Client) DoQuery(ctx context.Context, method, path string, query map[string][]string, out any) error {
	q := url.Values{}
	for k, vs := range query {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	return c.do(ctx, method, path, q, nil, out)
}

// HTTP exposes the underlying *http.Client so advanced callers can issue
// requests directly (e.g. for streaming downloads that bypass the JSON
// envelope). It is also used internally by SSE / NDJSON streams.
func (c *Client) HTTP() *http.Client { return c.http }

// NewStreamRequest constructs an *http.Request pre-loaded with the auth
// headers and the JSON body (if provided). The caller takes ownership of
// the request and is responsible for sending it via HTTP().
func (c *Client) NewStreamRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var bodyReader *strings.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("weknora: marshal request: %w", err)
		}
		bodyReader = strings.NewReader(string(buf))
	}
	var req *http.Request
	var err error
	if bodyReader != nil {
		req, err = http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("weknora: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.auth != nil {
		if err := c.auth.Apply(req); err != nil {
			return nil, fmt.Errorf("weknora: apply auth: %w", err)
		}
	}
	return req, nil
}

// DoRaw executes a caller-built *http.Request and decodes a JSON body.
// Useful for multipart uploads where the body has already been prepared.
func (c *Client) DoRaw(ctx context.Context, req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("weknora: http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return &APIError{
			StatusCode: resp.StatusCode,
			Code:       http.StatusText(resp.StatusCode),
			Message:    apiErr.Error,
		}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: decode body: %v", ErrInvalidResponse, err)
	}
	return nil
}

// NewIterator exposes the iterator constructor as a package-level function
// so service code can build iterators without importing the iterator type
// directly.
func NewIterator[T any](fetch func(ctx context.Context, token string) (Page[T], error)) *Iterator[T] {
	return &Iterator[T]{fetch: fetch}
}
