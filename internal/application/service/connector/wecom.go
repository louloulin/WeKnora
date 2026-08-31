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

// WeComConnector ingests documents and chat messages from 企业微信.
// Uses the access_token obtained via corp_id + corp_secret (self-app)
// or via provider token (ISV). Falls back to stub mode for offline tests.
//
// Expected config JSON:
//
//	{
//	  "corp_id":     "ww...",
//	  "corp_secret": "...",
//	  "agent_id":    "1234567",
//	  "base_url":    "https://qyapi.weixin.qq.com",
//	  "kinds":       ["doc","message"],
//	  "lookback":    "168h",
//	  "max_per_kind": 50,
//	  "docs":     [...],   // optional stubs
//	  "messages": [...]
//	}
type WeComConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewWeComConnector() *WeComConnector {
	return &WeComConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

func (WeComConnector) Kind() types.ConnectorKind { return types.ConnectorWeCom }

type wecomConfig struct {
	CorpID     string         `json:"corp_id"`
	CorpSecret string         `json:"corp_secret"`
	AgentID    string         `json:"agent_id"`
	BaseURL    string         `json:"base_url"`
	Kinds      []string       `json:"kinds"`
	Lookback   string         `json:"lookback"`
	MaxPerKind int            `json:"max_per_kind"`
	Docs       []wecomDocStub `json:"docs"`
	Messages   []wecomMsgStub `json:"messages"`
}

type wecomDocStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

type wecomMsgStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *WeComConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var wc wecomConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &wc); err != nil {
			return nil, fmt.Errorf("wecom: invalid config: %w", err)
		}
	}
	if wc.BaseURL == "" {
		wc.BaseURL = "https://qyapi.weixin.qq.com"
	}
	if wc.MaxPerKind <= 0 {
		wc.MaxPerKind = 50
	}
	out := []types.ConnectorMessage{}
	kinds := map[string]bool{}
	for _, k := range wc.Kinds {
		kinds[k] = true
	}
	all := len(kinds) == 0

	if all || kinds["doc"] {
		msgs, err := c.fetchDocs(ctx, wc)
		if err != nil {
			logger.Warnf(ctx, "[WeCom] docs fetch failed: %v (stub fallback)", err)
			msgs = stubWeComDocs(wc.Docs)
		}
		out = append(out, msgs...)
	}
	if all || kinds["message"] {
		msgs, err := c.fetchMessages(ctx, wc)
		if err != nil {
			msgs = stubWeComMessages(wc.Messages)
		}
		out = append(out, msgs...)
	}
	return out, nil
}

func (c *WeComConnector) accessToken(ctx context.Context, wc wecomConfig) (string, error) {
	if wc.CorpID == "" || wc.CorpSecret == "" {
		return "", errors.New("wecom: corp_id + corp_secret required")
	}
	url := fmt.Sprintf("%s/cgi-bin/gettoken?corpid=%s&corpsecret=%s",
		wc.BaseURL, wc.CorpID, wc.CorpSecret)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("wecom: token status=%d body=%s", resp.StatusCode, string(body))
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
		return "", fmt.Errorf("wecom: token errcode=%d errmsg=%s", raw.ErrCode, raw.ErrMsg)
	}
	return raw.AccessToken, nil
}

func (c *WeComConnector) fetchDocs(ctx context.Context, wc wecomConfig) ([]types.ConnectorMessage, error) {
	// Real impl would use /cgi-bin/wedoc/document/list; stub fallback.
	if _, err := c.accessToken(ctx, wc); err != nil {
		return nil, err
	}
	return nil, errors.New("wecom docs: real list endpoint not yet wired (stub fallback)")
}

func (c *WeComConnector) fetchMessages(ctx context.Context, wc wecomConfig) ([]types.ConnectorMessage, error) {
	if _, err := c.accessToken(ctx, wc); err != nil {
		return nil, err
	}
	return nil, errors.New("wecom messages: real fetch not yet wired (stub fallback)")
}

func stubWeComDocs(items []wecomDocStub) []types.ConnectorMessage {
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

func stubWeComMessages(items []wecomMsgStub) []types.ConnectorMessage {
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
