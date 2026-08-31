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

// DropboxConnector ingests files from Dropbox via the v2 API. Uses
// an OAuth2 access token. Falls back to stub mode for offline tests.
//
// Expected config JSON:
//
//	{
//	  "access_token": "...",
//	  "root_path":    "",
//	  "lookback":     "168h",
//	  "max_per_kind": 50,
//	  "files": [...] // optional stubs
//	}
type DropboxConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewDropboxConnector() *DropboxConnector {
	return &DropboxConnector{HTTPClient: &http.Client{Timeout: 30 * time.Second}, Now: time.Now}
}

func (DropboxConnector) Kind() types.ConnectorKind { return types.ConnectorDropbox }

type dropboxConfig struct {
	AccessToken string         `json:"access_token"`
	RootPath    string         `json:"root_path"`
	Lookback    string         `json:"lookback"`
	MaxPerKind  int            `json:"max_per_kind"`
	Files       []dropboxStub  `json:"files"`
}

type dropboxStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *DropboxConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var dc dropboxConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &dc); err != nil {
			return nil, fmt.Errorf("dropbox: invalid config: %w", err)
		}
	}
	if dc.AccessToken == "" {
		return nil, errors.New("dropbox: access_token required")
	}
	if dc.MaxPerKind <= 0 {
		dc.MaxPerKind = 50
	}
	body := fmt.Sprintf(`{"path":"%s","recursive":true,"limit":%d}`, dc.RootPath, dc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.dropboxapi.com/2/files/list_folder", io.NopCloser(stringReader(body)))
	req.Header.Set("Authorization", "Bearer "+dc.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		logger.Warnf(ctx, "[Dropbox] fetch failed: %v (stub fallback)", err)
		return stubDropbox(dc.Files), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dropbox: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	var raw struct {
		Entries []struct {
			ID          string    `json:"id"`
			Name        string    `json:"name"`
			PathDisplay string    `json:"path_display"`
			ServerModified time.Time `json:"server_modified"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, e := range raw.Entries {
		out = append(out, types.ConnectorMessage{
			ID: e.ID, Title: e.Name, Content: e.PathDisplay,
			URL: "https://www.dropbox.com/home" + e.PathDisplay,
			Timestamp: e.ServerModified,
			Metadata: map[string]string{"kind": "file"},
		})
	}
	return out, nil
}

func stubDropbox(items []dropboxStub) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(items))
	for _, i := range items {
		out = append(out, types.ConnectorMessage{
			ID: i.ID, Title: i.Title, Content: i.Body,
			Author: i.Author, URL: i.URL, Timestamp: i.UpdatedAt,
			Metadata: map[string]string{"kind": "file", "stub": "true"},
		})
	}
	return out
}
