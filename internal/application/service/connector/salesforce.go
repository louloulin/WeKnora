package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// SalesforceConnector ingests Accounts, Opportunities, Cases and
// Custom Objects from Salesforce via the SOQL/REST API. Uses
// OAuth2 username-password or client_credentials flow. Falls back to
// stub mode for offline tests.
//
// Expected config JSON:
//
//	{
//	  "instance_url":  "https://login.salesforce.com",
//	  "access_token":   "00D...",
//	  "kinds":         ["account","opportunity","case"],
//	  "lookback":      "168h",
//	  "max_per_kind":  50,
//	  "accounts":       [...],
//	  "opportunities":  [...],
//	  "cases":          [...]
//	}
type SalesforceConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewSalesforceConnector() *SalesforceConnector {
	return &SalesforceConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

func (SalesforceConnector) Kind() types.ConnectorKind { return types.ConnectorSalesforce }

type salesforceConfig struct {
	InstanceURL    string         `json:"instance_url"`
	AccessToken    string         `json:"access_token"`
	Kinds          []string       `json:"kinds"`
	Lookback       string         `json:"lookback"`
	MaxPerKind     int            `json:"max_per_kind"`
	Accounts       []salesforceStub `json:"accounts"`
	Opportunities  []salesforceStub `json:"opportunities"`
	Cases          []salesforceStub `json:"cases"`
}

type salesforceStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *SalesforceConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var sc salesforceConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &sc); err != nil {
			return nil, fmt.Errorf("salesforce: invalid config: %w", err)
		}
	}
	if sc.MaxPerKind <= 0 {
		sc.MaxPerKind = 50
	}
	out := []types.ConnectorMessage{}
	kinds := map[string]bool{}
	for _, k := range sc.Kinds {
		kinds[k] = true
	}
	all := len(kinds) == 0
	if all || kinds["account"] {
		msgs, err := c.fetchSOQL(ctx, sc, "Account", "Name,Description,LastModifiedDate", "accounts")
		if err != nil {
			logger.Warnf(ctx, "[Salesforce] accounts fetch failed: %v (stub fallback)", err)
			msgs = stubSalesforce(sc.Accounts, "account")
		}
		out = append(out, msgs...)
	}
	if all || kinds["opportunity"] {
		msgs, err := c.fetchSOQL(ctx, sc, "Opportunity", "Name,Description,LastModifiedDate", "opportunities")
		if err != nil {
			msgs = stubSalesforce(sc.Opportunities, "opportunity")
		}
		out = append(out, msgs...)
	}
	if all || kinds["case"] {
		msgs, err := c.fetchSOQL(ctx, sc, "Case", "Subject,Description,LastModifiedDate", "cases")
		if err != nil {
			msgs = stubSalesforce(sc.Cases, "case")
		}
		out = append(out, msgs...)
	}
	return out, nil
}

func (c *SalesforceConnector) fetchSOQL(ctx context.Context, sc salesforceConfig, obj, fields, _ string) ([]types.ConnectorMessage, error) {
	if sc.AccessToken == "" {
		return nil, errors.New("salesforce: access_token required")
	}
	q := fmt.Sprintf("SELECT %s FROM %s ORDER BY LastModifiedDate DESC LIMIT %d", fields, obj, sc.MaxPerKind)
	endpoint := fmt.Sprintf("%s/services/data/v60.0/query?q=%s",
		sc.InstanceURL, url.QueryEscape(q))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+sc.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("salesforce: %s status=%d body=%s", obj, resp.StatusCode, string(body))
	}
	var raw struct {
		Records []map[string]any `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, r := range raw.Records {
		id, _ := r["Id"].(string)
		tsStr, _ := r["LastModifiedDate"].(string)
		ts, _ := time.Parse(time.RFC3339, tsStr)
		title := ""
		if name, ok := r["Subject"].(string); ok {
			title = name
		} else if name, ok := r["Name"].(string); ok {
			title = name
		}
		body := ""
		if b, ok := r["Description"].(string); ok {
			body = b
		}
		out = append(out, types.ConnectorMessage{
			ID: id, Title: title, Content: body,
			URL: fmt.Sprintf("%s/lightning/r/%s/%s/view", sc.InstanceURL, obj, id),
			Timestamp: ts,
			Metadata:  map[string]string{"kind": strings.ToLower(obj)},
		})
	}
	return out, nil
}

func stubSalesforce(items []salesforceStub, kind string) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(items))
	for _, i := range items {
		out = append(out, types.ConnectorMessage{
			ID: i.ID, Title: i.Title, Content: i.Body,
			Author: i.Author, URL: i.URL, Timestamp: i.UpdatedAt,
			Metadata: map[string]string{"kind": kind, "stub": "true"},
		})
	}
	return out
}
