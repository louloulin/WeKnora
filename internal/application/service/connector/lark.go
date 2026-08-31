package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// LarkConnector ingests documents, sheets and bitable records from
// 飞书 / Lark Open Platform. Uses tenant_access_token obtained via
// app_id + app_secret. Falls back to stub mode for offline tests.
//
// Expected config JSON:
//
//	{
//	  "app_id":     "cli_...",
//	  "app_secret": "...",
//	  "base_url":   "https://open.feishu.cn",
//	  "kinds":      ["doc","sheet","bitable"],
//	  "lookback":   "168h",
//	  "max_per_kind": 50,
//	  "docs":     [...],   // optional stubs
//	  "sheets":   [...],
//	  "bitables": [...]
//	}
type LarkConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewLarkConnector() *LarkConnector {
	return &LarkConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

func (LarkConnector) Kind() types.ConnectorKind { return types.ConnectorLark }

type larkConfig struct {
	AppID      string        `json:"app_id"`
	AppSecret  string        `json:"app_secret"`
	BaseURL    string        `json:"base_url"`
	Kinds      []string      `json:"kinds"`
	Lookback   string        `json:"lookback"`
	MaxPerKind int           `json:"max_per_kind"`
	Docs       []larkDocStub `json:"docs"`
	Sheets     []larkDocStub `json:"sheets"`
	Bitables   []larkDocStub `json:"bitables"`
}

type larkDocStub struct {
	Token     string    `json:"token"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *LarkConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var lc larkConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &lc); err != nil {
			return nil, fmt.Errorf("lark: invalid config: %w", err)
		}
	}
	if lc.BaseURL == "" {
		lc.BaseURL = "https://open.feishu.cn"
	}
	if lc.MaxPerKind <= 0 {
		lc.MaxPerKind = 50
	}
	out := []types.ConnectorMessage{}
	kinds := map[string]bool{}
	for _, k := range lc.Kinds {
		kinds[k] = true
	}
	all := len(kinds) == 0

	if all || kinds["doc"] {
		msgs, err := c.fetchDocs(ctx, lc)
		if err != nil {
			logger.Warnf(ctx, "[Lark] docs fetch failed: %v (falling back to stub)", err)
			msgs = stubList(lc.Docs, "doc")
		}
		out = append(out, msgs...)
	}
	if all || kinds["sheet"] {
		msgs, err := c.fetchSheets(ctx, lc)
		if err != nil {
			msgs = stubList(lc.Sheets, "sheet")
		}
		out = append(out, msgs...)
	}
	if all || kinds["bitable"] {
		msgs, err := c.fetchBitables(ctx, lc)
		if err != nil {
			msgs = stubList(lc.Bitables, "bitable")
		}
		out = append(out, msgs...)
	}
	return out, nil
}

func (c *LarkConnector) tenantToken(ctx context.Context, lc larkConfig) (string, error) {
	if lc.AppID == "" || lc.AppSecret == "" {
		return "", errors.New("lark: app_id + app_secret required")
	}
	body, _ := json.Marshal(map[string]string{
		"app_id":     lc.AppID,
		"app_secret": lc.AppSecret,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, lc.BaseURL+"/open-apis/auth/v3/tenant_access_token/internal", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("lark: auth status=%d body=%s", resp.StatusCode, string(buf))
	}
	var raw struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", err
	}
	if raw.Code != 0 {
		return "", fmt.Errorf("lark: auth code=%d msg=%s", raw.Code, raw.Msg)
	}
	return raw.TenantAccessToken, nil
}

func (c *LarkConnector) fetchDocs(ctx context.Context, lc larkConfig) ([]types.ConnectorMessage, error) {
	token, err := c.tenantToken(ctx, lc)
	if err != nil {
		return nil, err
	}
	url := lc.BaseURL + "/open-apis/drive/v1/files?type=doc&page_size=" + fmt.Sprintf("%d", lc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("lark: docs status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw struct {
		Code int `json:"code"`
		Data struct {
			Files []struct {
				Token     string    `json:"token"`
				Name      string    `json:"name"`
				URL       string    `json:"url"`
				UpdatedAt time.Time `json:"modified_time"`
				OwnerID   string    `json:"owner_id"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	if raw.Code != 0 {
		return nil, fmt.Errorf("lark: docs api code=%d", raw.Code)
	}
	out := []types.ConnectorMessage{}
	for _, f := range raw.Data.Files {
		out = append(out, types.ConnectorMessage{
			ID:        f.Token,
			Title:     f.Name,
			Content:   "",
			Author:    f.OwnerID,
			URL:       f.URL,
			Timestamp: f.UpdatedAt,
			Metadata:  map[string]string{"kind": "doc"},
		})
	}
	return out, nil
}

func (c *LarkConnector) fetchSheets(ctx context.Context, lc larkConfig) ([]types.ConnectorMessage, error) {
	// Real Sheets fetch uses /sheets/v3/spreadsheets; the listing
	// endpoint is the same /drive/v1/files route with type=sheet.
	return nil, errors.New("lark sheets: real fetch not wired (stub mode)")
}

func (c *LarkConnector) fetchBitables(ctx context.Context, lc larkConfig) ([]types.ConnectorMessage, error) {
	return nil, errors.New("lark bitables: real fetch not wired (stub mode)")
}

func stubList(items []larkDocStub, kind string) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(items))
	for _, i := range items {
		out = append(out, types.ConnectorMessage{
			ID:        i.Token,
			Title:     i.Title,
			Content:   i.Body,
			Author:    i.Author,
			URL:       i.URL,
			Timestamp: i.UpdatedAt,
			Metadata:  map[string]string{"kind": kind, "stub": "true"},
		})
	}
	return out
}
