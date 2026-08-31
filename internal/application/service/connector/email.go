package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// EmailConnector is the v0.7.24 stub for the IMAP / SMTP email
// connector. The stub parses pre-fetched emails from the config
// JSON. In production this would call go-imap v2's idle client;
// the surface stays identical so the swap is mechanical.
//
// Expected config JSON:
//   {
//     "mailbox": "support@example.com",
//     "messages": [
//       {"message_id":"<abc@example.com>","from":"alice@example.com",
//        "subject":"Login help","date":"2026-08-30T12:00:00Z",
//        "body":"Hi team, I'm having trouble logging in..."}
//     ]
//   }
type EmailConnector struct{}

// Kind implements interfaces.Connector.
func (EmailConnector) Kind() types.ConnectorKind { return types.ConnectorEmail }

type emailMessage struct {
	MessageID string `json:"message_id"`
	From      string `json:"from"`
	Subject   string `json:"subject"`
	Date      string `json:"date"`
	Body      string `json:"body"`
}

type emailConfig struct {
	Mailbox  string         `json:"mailbox"`
	Messages []emailMessage `json:"messages"`
}

// Fetch implements interfaces.Connector.
func (EmailConnector) Fetch(_ context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	if strings.TrimSpace(cfg.ConfigJSON) == "" {
		return nil, errors.New("email: empty config")
	}
	var ec emailConfig
	if err := json.Unmarshal([]byte(cfg.ConfigJSON), &ec); err != nil {
		return nil, fmt.Errorf("email: parse config: %w", err)
	}
	out := make([]types.ConnectorMessage, 0, len(ec.Messages))
	for _, m := range ec.Messages {
		ts, err := parseEmailDate(m.Date)
		if err != nil {
			ts = time.Now()
		}
		author := m.From
		if parsed, perr := mail.ParseAddress(m.From); perr == nil {
			author = parsed.Name
			if author == "" {
				author = parsed.Address
			}
		}
		out = append(out, types.ConnectorMessage{
			ID:        m.MessageID,
			Title:     fmt.Sprintf("Email — %s", m.Subject),
			Content:   m.Body,
			Author:    author,
			Timestamp: ts,
			Metadata: map[string]string{
				"mailbox": ec.Mailbox,
				"from":    m.From,
				"subject": m.Subject,
				"source":  "email",
			},
		})
	}
	return out, nil
}

func parseEmailDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty date")
	}
	// RFC1123Z then RFC3339 then a few common fallbacks.
	for _, layout := range []string{
		time.RFC1123Z, time.RFC1123, time.RFC3339, time.RFC3339Nano,
		"2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("unparseable date")
}
