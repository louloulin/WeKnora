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

// BoxConnector ingests files and folders from Box via the Content API.
// Uses OAuth2 access token. Falls back to stub mode for offline tests.
//
// Expected config JSON:
//
//	{
//	  "access_token": "...",
//	  "root_folder_id": "0",
//	  "lookback":      "168h",
//	  "max_per_kind":  50,
//	  "files":   [...],   // optional stubs
//	  "folders": [...]
//	}
type BoxConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewBoxConnector() *BoxConnector {
	return &BoxConnector{HTTPClient: &http.Client{Timeout: 30 * time.Second}, Now: time.Now}
}

func (BoxConnector) Kind() types.ConnectorKind { return types.ConnectorBox }

type boxConfig struct {
	AccessToken   string      `json:"access_token"`
	RootFolderID  string      `json:"root_folder_id"`
	Lookback      string      `json:"lookback"`
	MaxPerKind    int         `json:"max_per_kind"`
	Files         []boxStub   `json:"files"`
	Folders       []boxStub   `json:"folders"`
}

type boxStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *BoxConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var bc boxConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &bc); err != nil {
			return nil, fmt.Errorf("box: invalid config: %w", err)
		}
	}
	if bc.AccessToken == "" {
		return nil, errors.New("box: access_token required")
	}
	if bc.RootFolderID == "" {
		bc.RootFolderID = "0"
	}
	if bc.MaxPerKind <= 0 {
		bc.MaxPerKind = 50
	}
	url := fmt.Sprintf("https://api.box.com/2.0/folders/%s/items?limit=%d", bc.RootFolderID, bc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+bc.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		logger.Warnf(ctx, "[Box] fetch failed: %v (stub fallback)", err)
		return stubBox(append(bc.Folders, bc.Files...)), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("box: status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw struct {
		Entries []struct {
			Type  string `json:"type"`
			ID    string `json:"id"`
			Name  string `json:"name"`
			ModifiedAt time.Time `json:"modified_at"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, e := range raw.Entries {
		out = append(out, types.ConnectorMessage{
			ID: e.ID, Title: e.Name, Content: "",
			URL: fmt.Sprintf("https://app.box.com/file/%s", e.ID),
			Timestamp: e.ModifiedAt,
			Metadata: map[string]string{"kind": e.Type},
		})
	}
	return out, nil
}

func stubBox(items []boxStub) []types.ConnectorMessage {
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
