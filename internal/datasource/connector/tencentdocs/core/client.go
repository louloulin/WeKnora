package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/datasource"
	"github.com/Tencent/WeKnora/internal/logger"
)

// Client wraps the Tencent Docs Open Platform API. It mirrors the Feishu
// client's surface (GetAccessToken / DoRequest / Download) so the per-type
// connectors (doc/...) can be written against an identical contract. The
// Feishu client is the authoritative reference for the retry / rate-limit /
// SSRF-guard knobs - keeping them aligned across connectors means new ones
// pick up the same operational behaviour for free.
type Client struct {
	baseURL string
	cfg     *Config

	// location renders timestamps in the user's timezone (default Asia/Shanghai).
	location *time.Location

	httpClient *http.Client

	// Token cache (thread-safe). OAuth2 access tokens are refreshed lazily.
	tokenMu    sync.Mutex
	tokenCache string
	tokenExpAt time.Time
}

// NewClient constructs a Tencent Docs API client from a validated Config.
// Outbound traffic goes through the SSRF-guarded shared HTTP client.
func NewClient(cfg *Config) *Client {
	return NewClientWithHTTPClient(cfg, datasource.NewConnectorHTTPClient(30*time.Second))
}

// NewClientWithHTTPClient is the explicit constructor used by tests that need
// to inject a custom *http.Client (e.g. an httptest.Server.Client()). The
// production path goes through NewClient; this variant exists so the same
// code path is exercised without dragging the SSRF guard into unit tests.
//
// Production callers MUST keep using NewClient so the SSRF / redirect /
// dial-time protections stay active.
func NewClientWithHTTPClient(cfg *Config, httpClient *http.Client) *Client {
	return &Client{
		baseURL:    cfg.GetBaseURL(),
		cfg:        cfg,
		location:   resolveLocation(cfg.Timezone),
		httpClient: httpClient,
	}
}

// tz returns the client's rendering timezone, defaulting to Asia/Shanghai when
// the Client was built without one (e.g. in tests that construct directly).
func (c *Client) tz() *time.Location {
	if c.location != nil {
		return c.location
	}
	return time.FixedZone("CST", 8*3600)
}

// Location exposes the client's timezone so per-type connectors can format
// timestamps without re-parsing the config.
func (c *Client) Location() *time.Location { return c.tz() }

// Config returns the parsed config so adapters can read client_id, base URL
// overrides and resource IDs without re-parsing.
func (c *Client) Config() *Config { return c.cfg }

// Retry policy shared by DoRequest and downloadRawBytes: 429 honours
// Retry-After, 5xx retries once, transport errors back off with the same
// schedule as the Feishu client so operators can predict behaviour.
const (
	tencentDocsMaxRetries    = 3
	tencentDocsMax5xxRetries = 1
	tencentDocsRetry5xxDelay = 2 * time.Second
)

// maxTencentDocsDownloadBytes bounds a single file download (slides export,
// sheet export, attachment fetch) to protect the sync worker from adversarial
// or pathological oversized responses.
const maxTencentDocsDownloadBytes = 512 * 1024 * 1024 // 512 MB

var tencentDocsRetryBackoff = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}

// GetAccessToken returns a cached or freshly-fetched OAuth2 access token.
// When the config already supplies a long-lived AccessToken we use it
// verbatim and skip the OAuth round-trip. Otherwise we exchange
// client_id/client_secret via the /oauth/v2/token endpoint and cache the
// result with a 5-minute safety margin against the issued expires_in.
func (c *Client) GetAccessToken(ctx context.Context) (string, error) {
	if c.cfg != nil && c.cfg.AccessToken != "" {
		return c.cfg.AccessToken, nil
	}

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.tokenCache != "" && time.Now().Before(c.tokenExpAt) {
		return c.tokenCache, nil
	}

	form := strings.NewReader(urlEncode(map[string]string{
		"client_id":     c.cfg.ClientID,
		"client_secret": c.cfg.ClientSecret,
		"grant_type":    "client_credentials",
		"scope":         "doc:read drive:read sheet:read slide:read",
	}))

	url := c.baseURL + "/oauth/v2/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, form)
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request token: %w", err)
	}
	defer resp.Body.Close()

	var result TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("tencent_docs auth error: %s: %s", result.Error, result.ErrorDesc)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("tencent_docs auth error: empty access_token")
	}

	c.tokenCache = result.AccessToken
	ttl := time.Duration(result.ExpiresIn) * time.Second
	if ttl > 5*time.Minute {
		ttl -= 5 * time.Minute
	}
	c.tokenExpAt = time.Now().Add(ttl)

	prefixLen := 8
	if len(result.AccessToken) < prefixLen {
		prefixLen = len(result.AccessToken)
	}
	suffixLen := 4
	if len(result.AccessToken) < suffixLen {
		suffixLen = len(result.AccessToken)
	}
	logger.Infof(ctx, "[TencentDocs] got access_token: %s...%s expires_in=%ds",
		result.AccessToken[:prefixLen], result.AccessToken[len(result.AccessToken)-suffixLen:], result.ExpiresIn)

	return c.tokenCache, nil
}

// DoRequest executes an authenticated API request and decodes the JSON
// response, retrying transient failures (transport errors, HTTP 429, 5xx).
// 429 honours Retry-After; 5xx retries once; other 4xx fails fast.
func (c *Client) DoRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	token, err := c.GetAccessToken(ctx)
	if err != nil {
		return err
	}

	var bodyBytes []byte
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
	}

	url := c.baseURL + path
	var lastErr error

	for attempt := 0; attempt <= tencentDocsMaxRetries; attempt++ {
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")

		if attempt == 0 {
			logger.Infof(ctx, "[TencentDocs] %s %s", method, path)
		} else {
			logger.Infof(ctx, "[TencentDocs] %s %s (retry %d/%d)", method, path, attempt, tencentDocsMaxRetries)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("execute request: %w", err)
			if attempt < tencentDocsMaxRetries {
				if sErr := sleepCtx(ctx, tencentDocsRetryBackoff[attempt]); sErr != nil {
					return sErr
				}
				continue
			}
			return lastErr
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read response body: %w", readErr)
			if attempt < tencentDocsMaxRetries {
				if sErr := sleepCtx(ctx, tencentDocsRetryBackoff[attempt]); sErr != nil {
					return sErr
				}
				continue
			}
			return lastErr
		}

		logger.Infof(ctx, "[TencentDocs] %s %s -> status=%d bodyLen=%d body=%s",
			method, path, resp.StatusCode, len(respBody), truncate(string(respBody), 1000))

		if resp.StatusCode == http.StatusTooManyRequests {
			wait := parseRetryAfter(resp.Header.Get("Retry-After"), tencentDocsRetryBackoff[minInt(attempt, len(tencentDocsRetryBackoff)-1)])
			lastErr = fmt.Errorf("tencent_docs rate limited: status=429 body=%s", truncate(string(respBody), 500))
			if attempt < tencentDocsMaxRetries {
				if sErr := sleepCtx(ctx, wait); sErr != nil {
					return sErr
				}
				continue
			}
			return lastErr
		}

		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			lastErr = fmt.Errorf("tencent_docs server error: status=%d body=%s", resp.StatusCode, truncate(string(respBody), 500))
			if attempt < tencentDocsMax5xxRetries {
				if sErr := sleepCtx(ctx, tencentDocsRetry5xxDelay); sErr != nil {
					return sErr
				}
				continue
			}
			return lastErr
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("tencent_docs api error: status=%d body=%s", resp.StatusCode, string(respBody))
		}

		if result != nil {
			if err := json.Unmarshal(respBody, result); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
		}
		return nil
	}

	return lastErr
}

// Download fetches a binary blob (sheet export, slide export, attachment) with
// the same retry policy as DoRequest but without JSON decoding.
func (c *Client) Download(ctx context.Context, url string) ([]byte, error) {
	token, err := c.GetAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt <= tencentDocsMaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("create download request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("download: %w", err)
			if attempt < tencentDocsMaxRetries {
				if sErr := sleepCtx(ctx, tencentDocsRetryBackoff[attempt]); sErr != nil {
					return nil, sErr
				}
				continue
			}
			return nil, lastErr
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			wait := parseRetryAfter(resp.Header.Get("Retry-After"), tencentDocsRetryBackoff[minInt(attempt, len(tencentDocsRetryBackoff)-1)])
			lastErr = fmt.Errorf("tencent_docs rate limited: status=429")
			if attempt < tencentDocsMaxRetries {
				if sErr := sleepCtx(ctx, wait); sErr != nil {
					return nil, sErr
				}
				continue
			}
			return nil, lastErr
		}
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			resp.Body.Close()
			lastErr = fmt.Errorf("tencent_docs server error: status=%d", resp.StatusCode)
			if attempt < tencentDocsMax5xxRetries {
				if sErr := sleepCtx(ctx, tencentDocsRetry5xxDelay); sErr != nil {
					return nil, sErr
				}
				continue
			}
			return nil, lastErr
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("tencent_docs download error: status=%d body=%s", resp.StatusCode, string(body))
		}
		defer resp.Body.Close()

		limited := io.LimitReader(resp.Body, maxTencentDocsDownloadBytes+1)
		buf, readErr := io.ReadAll(limited)
		if readErr != nil {
			return nil, fmt.Errorf("read download body: %w", readErr)
		}
		if int64(len(buf)) > maxTencentDocsDownloadBytes {
			return nil, fmt.Errorf("tencent_docs download exceeds %d bytes", maxTencentDocsDownloadBytes)
		}
		return buf, nil
	}
	return nil, lastErr
}

// Ping verifies the credentials by calling a lightweight API (token endpoint
// or a profile-style "me" endpoint). Implemented in PingMe below.
func (c *Client) Ping(ctx context.Context) error {
	if c.cfg != nil && c.cfg.AccessToken != "" {
		// Static token path: trust it but issue a cheap drive listing call so
		// 401s surface at Validate time instead of mid-sync.
		return c.DoRequest(ctx, http.MethodGet, "/drive/v2/files?page_size=1", nil, nil)
	}
	_, err := c.GetAccessToken(ctx)
	return err
}

// ---------- helpers (verbatim copy of the Feishu utility trio: parseRetryAfter,
// sleepCtx, truncate). Kept local so this package has no compile-time
// dependency on feishu/core - the engine reuse is via the NodeOps adapter
// only, and we want the option to vendor or extract those utilities later.

// parseRetryAfter interprets a Retry-After header (seconds) into a wait
// duration, coercing 0/negative to a short delay and falling back when
// absent or unparseable.
func parseRetryAfter(header string, fallback time.Duration) time.Duration {
	if header == "" {
		return fallback
	}
	secs, err := strconv.ParseFloat(strings.TrimSpace(header), 64)
	if err != nil {
		return fallback
	}
	if secs <= 0 {
		return 100 * time.Millisecond
	}
	return time.Duration(secs * float64(time.Second))
}

// sleepCtx waits for d or until ctx is cancelled, returning ctx.Err() so
// retries abort promptly on task cancellation / timeout.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// truncate truncates a string to maxLen and appends "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// urlEncode is a minimal form-encoder so we don't pull in net/url just for
// the OAuth token request. Strings only - matches the OAuth2 token contract.
func urlEncode(values map[string]string) string {
	parts := make([]string, 0, len(values))
	for k, v := range values {
		parts = append(parts, urlEncodeEscaped(k)+"="+urlEncodeEscaped(v))
	}
	return strings.Join(parts, "&")
}

func urlEncodeEscaped(s string) string {
	const safe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
	var b strings.Builder
	for _, r := range s {
		if r <= 0x7F && strings.ContainsRune(safe, r) {
			b.WriteRune(r)
		} else {
			for _, x := range []byte(string(r)) {
				b.WriteByte('%')
				b.WriteString(strings.ToUpper(strconv.FormatUint(uint64(x), 16)))
			}
		}
	}
	return b.String()
}

// resolveLocation mirrors the Feishu helper - parses a TZ name with a safe
// fallback to Asia/Shanghai. Kept here rather than imported to avoid the
// cross-package dependency on feishu/core.
func resolveLocation(tz string) *time.Location {
	if tz == "" {
		return time.FixedZone("CST", 8*3600)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}

// ---------- Tencent Docs API methods ----------
//
// These wrappers own the wire format for the three endpoints the doc
// connector relies on. They are deliberately thin: input validation +
// DoRequest + decode. Tenant / pagination / retry all live in the lower
// layer so adding new endpoints only needs a few lines here.

// DriveListResponse is the /drive/v2/files reply envelope.
type DriveListResponse struct {
	ErrCode int       `json:"errcode"`
	ErrMsg  string    `json:"errmsg"`
	Data    DriveData `json:"data"`
}

// DriveData is the inner data payload. Files is the document slice; HasMore
// + Next drive the pagination loop.
type DriveData struct {
	Files   []Document `json:"files"`
	HasMore bool       `json:"has_more"`
	Next    string     `json:"next"`
	Total   int        `json:"total"`
}

// ListDriveFiles returns up to limit documents of the given doc type
// (doc / sheet / slide / form) accessible to the configured app. When
// parentID is empty the call hits the root drive listing.
func (c *Client) ListDriveFiles(ctx context.Context, docType string, limit int) ([]Document, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var all []Document
	cursor := ""
	for {
		path := fmt.Sprintf("/drive/v2/files?type=%s&page_size=%d", docType, limit)
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}

		var resp DriveListResponse
		if err := c.DoRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
			return nil, fmt.Errorf("list drive files: %w", err)
		}
		if resp.ErrCode != 0 {
			return nil, fmt.Errorf("list drive files: errcode=%d errmsg=%s", resp.ErrCode, resp.ErrMsg)
		}

		all = append(all, resp.Data.Files...)
		if !resp.Data.HasMore || resp.Data.Next == "" {
			break
		}
		cursor = resp.Data.Next
	}
	return all, nil
}

// ListDriveFilesRecursive resolves a resource ID to its contained documents.
// For the picker that means "the drive folder ID" → all docs of the
// requested type. For a single-document resource ID it returns a one-element
// slice so the engine treats it as a sync of one doc.
func (c *Client) ListDriveFilesRecursive(ctx context.Context, resourceID, docType string, limit int) ([]Document, error) {
	if resourceID == "" {
		return nil, fmt.Errorf("tencent_docs: empty resourceID")
	}
	// For v1 we treat the resource as either a drive root or a single
	// document ID. If it looks like a single document (no folder semantics)
	// return a one-element slice so the engine still issues a Fetch.
	if looksLikeDocumentID(resourceID) {
		d, err := c.GetDocument(ctx, resourceID)
		if err != nil {
			return nil, err
		}
		return []Document{d}, nil
	}
	return c.ListDriveFiles(ctx, docType, limit)
}

// DocumentResponse is the reply from /doc/v3/{id}/info.
type DocumentResponse struct {
	ErrCode int      `json:"errcode"`
	ErrMsg  string   `json:"errmsg"`
	Data    Document `json:"data"`
}

// GetDocument returns metadata for a single document by ID.
func (c *Client) GetDocument(ctx context.Context, docID string) (Document, error) {
	path := fmt.Sprintf("/doc/v3/%s/info", url.PathEscape(docID))
	var resp DocumentResponse
	if err := c.DoRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return Document{}, fmt.Errorf("get document: %w", err)
	}
	if resp.ErrCode != 0 {
		return Document{}, fmt.Errorf("get document: errcode=%d errmsg=%s", resp.ErrCode, resp.ErrMsg)
	}
	d := resp.Data
	if d.ID == "" {
		d.ID = docID
	}
	if d.URL == "" {
		d.URL = WebDocURL(docID)
	}
	return d, nil
}

// DocumentContentResponse is the reply from /doc/v3/{id}/content. The
// ContentType mirrors the renderer the API used to serialise the doc
// (markdown / html / raw) so downstream consumers can route appropriately.
type DocumentContentResponse struct {
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
	Data         string `json:"data"`
	ContentType  string `json:"content_type"`
	RevisionTime string `json:"revision_time"`
}

// FetchDocumentContent retrieves a document's textual content. Returns
// (content, contentType, error). When the API exposes markdown natively the
// content_type is "text/markdown"; otherwise "text/html" and the caller can
// convert. Empty contentType means "unknown - treat as plain text".
func (c *Client) FetchDocumentContent(ctx context.Context, docID string) (string, string, error) {
	path := fmt.Sprintf("/doc/v3/%s/content", url.PathEscape(docID))
	var resp DocumentContentResponse
	if err := c.DoRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return "", "", fmt.Errorf("fetch document content: %w", err)
	}
	if resp.ErrCode != 0 {
		return "", "", fmt.Errorf("fetch document content: errcode=%d errmsg=%s", resp.ErrCode, resp.ErrMsg)
	}
	return resp.Data, resp.ContentType, nil
}

// looksLikeDocumentID is a best-effort heuristic: Tencent Docs doc IDs are
// 16-22 char alphanumeric strings starting with a type letter (D / S / B /
// F). Anything shorter or with non-alphanumeric characters is treated as a
// folder / drive ID.
func looksLikeDocumentID(id string) bool {
	if len(id) < 8 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}
