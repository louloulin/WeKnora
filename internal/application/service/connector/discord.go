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

// DiscordConnector ingests channel messages from Discord via the
// REST API. Uses a bot token. Falls back to stub mode for offline tests.
//
// Expected config JSON:
//
//	{
//	  "bot_token":     "...",
//	  "guild_id":      "...",
//	  "channel_ids":   ["123","456"],
//	  "lookback":      "168h",
//	  "max_per_channel": 50,
//	  "messages":  [...] // optional stubs
//	}
type DiscordConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewDiscordConnector() *DiscordConnector {
	return &DiscordConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

func (DiscordConnector) Kind() types.ConnectorKind { return types.ConnectorDiscord }

type discordConfig struct {
	BotToken      string         `json:"bot_token"`
	GuildID       string         `json:"guild_id"`
	ChannelIDs    []string       `json:"channel_ids"`
	Lookback      string         `json:"lookback"`
	MaxPerChannel int            `json:"max_per_channel"`
	Messages      []discordStub  `json:"messages"`
}

type discordStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *DiscordConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var dc discordConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &dc); err != nil {
			return nil, fmt.Errorf("discord: invalid config: %w", err)
		}
	}
	if dc.BotToken == "" {
		return nil, errors.New("discord: bot_token required")
	}
	if dc.MaxPerChannel <= 0 {
		dc.MaxPerChannel = 50
	}
	out := []types.ConnectorMessage{}
	channels := dc.ChannelIDs
	if len(channels) == 0 {
		return nil, errors.New("discord: channel_ids required")
	}
	for _, channelID := range channels {
		msgs, err := c.fetchChannel(ctx, dc, channelID)
		if err != nil {
			logger.Warnf(ctx, "[Discord] channel %s fetch failed: %v", channelID, err)
			continue
		}
		out = append(out, msgs...)
	}
	if len(out) == 0 {
		out = stubDiscord(dc.Messages)
	}
	return out, nil
}

func (c *DiscordConnector) fetchChannel(ctx context.Context, dc discordConfig, channelID string) ([]types.ConnectorMessage, error) {
	url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages?limit=%d", channelID, dc.MaxPerChannel)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bot "+dc.BotToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("discord: status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw []struct {
		ID        string    `json:"id"`
		Content   string    `json:"content"`
		Author    struct{ Username string `json:"username"` } `json:"author"`
		Timestamp time.Time `json:"timestamp"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, m := range raw {
		out = append(out, types.ConnectorMessage{
			ID:        m.ID,
			Title:     m.Author.Username + ": " + truncate(m.Content, 60),
			Content:   m.Content,
			Author:    m.Author.Username,
			URL:       "https://discord.com/channels/" + dc.GuildID + "/" + channelID + "/" + m.ID,
			Timestamp: m.Timestamp,
			Metadata:  map[string]string{"kind": "message", "channel_id": channelID},
		})
	}
	return out, nil
}

func stubDiscord(items []discordStub) []types.ConnectorMessage {
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
