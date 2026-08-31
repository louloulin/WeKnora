package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// DingTalkConnector ingests documents and group messages from 钉钉.
// Uses the access_token obtained via appKey + appSecret. Falls back
// to stub mode for offline tests.
//
// Expected config JSON:
//
//	{
//	  "app_key":    "...",
//	  "app_secret": "...",
//	  "agent_id":   "123",
//	  "base_url":   "https://oapi.dingtalk.com",
//	  "kinds":      ["doc","message"],
//	  "lookback":   "168h",
//	  "max_per_kind": 50,
//	  "docs":     [...],  // optional stubs
//	  "messages": [...]
//	}
type DingTalkConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewDingTalkConnector() *DingTalkConnector {
	return &DingTalkConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

func (DingTalkConnector) Kind() types.ConnectorKind { return types.ConnectorDingTalk }

type dingtalkConfig struct {
	AppKey     string             `json:"app_key"`
	AppSecret  string             `json:"app_secret"`
	AgentID    string             `json:"agent_id"`
	BaseURL    string             `json:"base_url"`
	Kinds      []string           `json:"kinds"`
	Lookback   string             `json:"lookback"`
	MaxPerKind int                `json:"max_per_kind"`
	Docs       []dingtalkDocStub  `json:"docs"`
	Messages   []dingtalkMsgStub  `json:"messages"`
}

type dingtalkDocStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

type dingtalkMsgStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *DingTalkConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var dc dingtalkConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &dc); err != nil {
			return nil, fmt.Errorf("dingtalk: invalid config: %w", err)
		}
	}
	if dc.BaseURL == "" {
		dc.BaseURL = "https://oapi.dingtalk.com"
	}
	if dc.MaxPerKind <= 0 {
		dc.MaxPerKind = 50
	}
	out := []types.ConnectorMessage{}
	kinds := map[string]bool{}
	for _, k := range dc.Kinds {
		kinds[k] = true
	}
	all := len(kinds) == 0

	if all || kinds["doc"] {
		msgs, err := c.fetchDocs(ctx, dc)
		if err != nil {
			logger.Warnf(ctx, "[DingTalk] docs fetch failed: %v (stub fallback)", err)
			msgs = stubDocs(dc.Docs)
		}
		out = append(out, msgs...)
	}
	if all || kinds["message"] {
		msgs, err := c.fetchMessages(ctx, dc)
		if err != nil {
			msgs = stubMessages(dc.Messages)
		}
		out = append(out, msgs...)
	}
	return out, nil
}

func (c *DingTalkConnector) accessToken(ctx context.Context, dc dingtalkConfig) (string, error) {
	if dc.AppKey == "" || dc.AppSecret == "" {
		return "", errors.New("dingtalk: app_key + app_secret required")
	}
	url := fmt.Sprintf("%s/gettoken?appkey=%s&appsecret=%s", dc.BaseURL, dc.AppKey, dc.AppSecret)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("dingtalk: token status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", err
	}
	if raw.ErrCode != 0 {
		return "", fmt.Errorf("dingtalk: token errcode=%d errmsg=%s", raw.ErrCode, raw.ErrMsg)
	}
	return raw.AccessToken, nil
}

func (c *DingTalkConnector) fetchDocs(ctx context.Context, dc dingtalkConfig) ([]types.ConnectorMessage, error) {
	// Real impl uses /topapi/docs/list; stub fallback for now.
	_, err := c.accessToken(ctx, dc)
	if err != nil {
		return nil, err
	}
	return nil, errors.New("dingtalk docs: real list endpoint not yet wired (stub fallback)")
}

func (c *DingTalkConnector) fetchMessages(ctx context.Context, dc dingtalkConfig) ([]types.ConnectorMessage, error) {
	_, err := c.accessToken(ctx, dc)
	if err != nil {
		return nil, err
	}
	return nil, errors.New("dingtalk messages: real fetch not yet wired (stub fallback)")
}

func stubDocs(items []dingtalkDocStub) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(items))
	for _, i := range items {
		out = append(out, types.ConnectorMessage{
			ID:        i.ID,
			Title:     i.Title,
			Content:   i.Body,
			Author:    i.Author,
			URL:       i.URL,
			Timestamp: i.UpdatedAt,
			Metadata:  map[string]string{"kind": "doc", "stub": "true"},
		})
	}
	return out
}

func stubMessages(items []dingtalkMsgStub) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(items))
	for _, i := range items {
		out = append(out, types.ConnectorMessage{
			ID:        i.ID,
			Title:     i.Title,
			Content:   i.Body,
			Author:    i.Author,
			URL:       i.URL,
			Timestamp: i.UpdatedAt,
			Metadata:  map[string]string{"kind": "message", "stub": "true"},
		})
	}
	return out
}
