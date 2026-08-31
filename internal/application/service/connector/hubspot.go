package connector

import (
	"strings"
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

// HubSpotConnector ingests Contacts, Companies and Tickets from
// HubSpot via the CRM API. Uses a private app token. Falls back to
// stub mode for offline tests.
//
// Expected config JSON:
//
//	{
//	  "access_token": "pat-...",
//	  "kinds":        ["contact","company","ticket"],
//	  "lookback":     "168h",
//	  "max_per_kind": 50,
//	  "contacts":  [...],   // optional stubs
//	  "companies": [...],
//	  "tickets":   [...]
//	}
type HubSpotConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewHubSpotConnector() *HubSpotConnector {
	return &HubSpotConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

func (HubSpotConnector) Kind() types.ConnectorKind { return types.ConnectorHubSpot }

type hubspotConfig struct {
	AccessToken string         `json:"access_token"`
	Kinds       []string       `json:"kinds"`
	Lookback    string         `json:"lookback"`
	MaxPerKind  int            `json:"max_per_kind"`
	Contacts    []hubspotStub  `json:"contacts"`
	Companies   []hubspotStub  `json:"companies"`
	Tickets     []hubspotStub  `json:"tickets"`
}

type hubspotStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *HubSpotConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var hc hubspotConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &hc); err != nil {
			return nil, fmt.Errorf("hubspot: invalid config: %w", err)
		}
	}
	if hc.MaxPerKind <= 0 {
		hc.MaxPerKind = 50
	}
	out := []types.ConnectorMessage{}
	kinds := map[string]bool{}
	for _, k := range hc.Kinds {
		kinds[k] = true
	}
	all := len(kinds) == 0
	if all || kinds["contact"] {
		msgs, err := c.fetchContacts(ctx, hc)
		if err != nil {
			logger.Warnf(ctx, "[HubSpot] contacts fetch failed: %v (stub fallback)", err)
			msgs = stubHubSpot(hc.Contacts, "contact")
		}
		out = append(out, msgs...)
	}
	if all || kinds["company"] {
		msgs, err := c.fetchCompanies(ctx, hc)
		if err != nil {
			msgs = stubHubSpot(hc.Companies, "company")
		}
		out = append(out, msgs...)
	}
	if all || kinds["ticket"] {
		msgs, err := c.fetchTickets(ctx, hc)
		if err != nil {
			msgs = stubHubSpot(hc.Tickets, "ticket")
		}
		out = append(out, msgs...)
	}
	return out, nil
}

func (c *HubSpotConnector) fetchContacts(ctx context.Context, hc hubspotConfig) ([]types.ConnectorMessage, error) {
	if hc.AccessToken == "" {
		return nil, errors.New("hubspot: access_token required")
	}
	url := fmt.Sprintf("https://api.hubapi.com/crm/v3/objects/contacts?limit=%d", hc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+hc.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hubspot: contacts status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw struct {
		Results []struct {
			ID        string `json:"id"`
			UpdatedAt string `json:"updatedAt"`
			Properties struct {
				Firstname string `json:"firstname"`
				Lastname  string `json:"lastname"`
				Email     string `json:"email"`
			} `json:"properties"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, r := range raw.Results {
		title := strings.TrimSpace(r.Properties.Firstname + " " + r.Properties.Lastname)
		if title == "" {
			title = r.Properties.Email
		}
		ts, _ := time.Parse(time.RFC3339, r.UpdatedAt)
		out = append(out, types.ConnectorMessage{
			ID: r.ID, Title: title, Content: r.Properties.Email,
			Author: r.Properties.Email, URL: "https://app.hubspot.com/contacts/" + r.ID,
			Timestamp: ts, Metadata: map[string]string{"kind": "contact"},
		})
	}
	return out, nil
}

func (c *HubSpotConnector) fetchCompanies(ctx context.Context, hc hubspotConfig) ([]types.ConnectorMessage, error) {
	if hc.AccessToken == "" {
		return nil, errors.New("hubspot: access_token required")
	}
	url := fmt.Sprintf("https://api.hubapi.com/crm/v3/objects/companies?limit=%d", hc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+hc.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hubspot: companies status=%d", resp.StatusCode)
	}
	var raw struct {
		Results []struct {
			ID        string `json:"id"`
			UpdatedAt string `json:"updatedAt"`
			Properties struct {
				Name string `json:"name"`
			} `json:"properties"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, r := range raw.Results {
		ts, _ := time.Parse(time.RFC3339, r.UpdatedAt)
		out = append(out, types.ConnectorMessage{
			ID: r.ID, Title: r.Properties.Name, Content: "",
			URL: "https://app.hubspot.com/companies/" + r.ID,
			Timestamp: ts, Metadata: map[string]string{"kind": "company"},
		})
	}
	return out, nil
}

func (c *HubSpotConnector) fetchTickets(ctx context.Context, hc hubspotConfig) ([]types.ConnectorMessage, error) {
	if hc.AccessToken == "" {
		return nil, errors.New("hubspot: access_token required")
	}
	url := fmt.Sprintf("https://api.hubapi.com/crm/v3/objects/tickets?limit=%d", hc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+hc.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hubspot: tickets status=%d", resp.StatusCode)
	}
	var raw struct {
		Results []struct {
			ID        string `json:"id"`
			UpdatedAt string `json:"updatedAt"`
			Properties struct {
				Subject     string `json:"subject"`
				Content     string `json:"content"`
			} `json:"properties"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, r := range raw.Results {
		ts, _ := time.Parse(time.RFC3339, r.UpdatedAt)
		out = append(out, types.ConnectorMessage{
			ID: r.ID, Title: r.Properties.Subject, Content: r.Properties.Content,
			URL: "https://app.hubspot.com/tickets/" + r.ID,
			Timestamp: ts, Metadata: map[string]string{"kind": "ticket"},
		})
	}
	return out, nil
}

func stubHubSpot(items []hubspotStub, kind string) []types.ConnectorMessage {
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

// strings import — added at file bottom to avoid touching every section.

