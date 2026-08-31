package connector

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// knowledgeIngester is the v0.7.25 Build #25 (G05) real KnowledgeIngester
// implementation that bridges the AI Connector framework into the
// existing KB / chunk / embedding pipeline.
//
// Per-message ingest path:
//
//  1. Build a Markdown payload (title + author + source link + body).
//  2. Resolve tenantID (string from the runtime config) to uint64 —
//     the value is supplied by the handler and we treat it as the
//     canonical tenant id; when conversion fails the connector is
//     rejected as misconfigured.
//  3. Call KnowledgeManualCreator.CreateKnowledgeFromManual so the
//     existing draft/publish pipeline, the chunker, the embedder
//     and the vector store all run unchanged. The connector just
//     hands the message off as a "manual" entry tagged with the
//     connector kind.
//
// If the underlying creator is nil the ingester returns ErrNoIngester;
// the connector handler turns this into an HTTP 503 so callers
// know the deployment is mis-wired rather than silently dropping
// messages.
//
// We depend on a narrow interface (KnowledgeManualCreator) rather
// than the full KnowledgeService to keep unit tests trivial — the
// production wiring in container.go injects the real service,
// which satisfies the narrower contract automatically.
type KnowledgeManualCreator interface {
	CreateKnowledgeFromManual(ctx context.Context, kbID string, payload *types.ManualKnowledgePayload, channel string) (*types.Knowledge, error)
}

type knowledgeIngester struct {
	ks  KnowledgeManualCreator
	now func() time.Time
}

// NewKnowledgeIngester wires the real ingester. Pass nil for the
// service to get the v0.7.24 "ingest is a no-op" behavior, useful
// when the platform is running lite / without a knowledge pipeline.
func NewKnowledgeIngester(ks KnowledgeManualCreator) KnowledgeIngester {
	if ks == nil {
		return &knowledgeIngester{ks: nil, now: time.Now}
	}
	return &knowledgeIngester{ks: ks, now: time.Now}
}

// ErrNoIngester is returned by Ingest when the runtime is missing
// the underlying KnowledgeService dependency.
var ErrNoIngester = errors.New("connector: knowledge pipeline not wired (ks is nil)")

// Ingest implements KnowledgeIngester.
func (k *knowledgeIngester) Ingest(ctx context.Context, tenantIDStr, kbID, title, content, author, url string, ts time.Time) error {
	if k.ks == nil {
		return ErrNoIngester
	}
	if strings.TrimSpace(tenantIDStr) == "" {
		return errors.New("connector: tenant_id required")
	}
	if strings.TrimSpace(kbID) == "" {
		return errors.New("connector: knowledge_base_id required")
	}

	tenantID, err := strconv.ParseUint(tenantIDStr, 10, 64)
	if err != nil {
		// Treat the string as already-numeric — fallback path for the
		// legacy test fixtures that pass "1" / "tenant-1".
		// Keep it strict-ish: if it doesn't parse, fail loudly so
		// production deploys catch misconfiguration.
		return fmt.Errorf("connector: tenant_id %q is not a uint64: %w", tenantIDStr, err)
	}

	body := buildConnectorMarkdown(title, content, author, url, ts)
	if title == "" {
		title = fmt.Sprintf("Connector ingest — %s", ts.Format(time.RFC3339))
	}
	payload := &types.ManualKnowledgePayload{
		Title:   title,
		Content: body,
		Status:  types.ManualKnowledgeStatusPublish,
		Channel: "connector",
	}

	if _, err := k.ks.CreateKnowledgeFromManual(ctx, kbID, payload, "connector"); err != nil {
		return fmt.Errorf("connector: CreateKnowledgeFromManual failed: %w", err)
	}

	logger.Infof(ctx, "connector: ingested message (tenant=%d kb=%s title=%q author=%q)",
		tenantID, kbID, title, author)
	return nil
}

// buildConnectorMarkdown wraps the connector message in a stable
// Markdown shape so the downstream chunker sees consistent content.
// Author + source URL become front-matter so search-time citations
// can surface them.
func buildConnectorMarkdown(title, content, author, url string, ts time.Time) string {
	var b strings.Builder
	b.WriteString("---\n")
	if title != "" {
		b.WriteString("title: ")
		b.WriteString(quoteYAML(title))
		b.WriteString("\n")
	}
	if author != "" {
		b.WriteString("author: ")
		b.WriteString(quoteYAML(author))
		b.WriteString("\n")
	}
	if url != "" {
		b.WriteString("source: ")
		b.WriteString(url)
		b.WriteString("\n")
	}
	if !ts.IsZero() {
		b.WriteString("ingested_at: ")
		b.WriteString(ts.UTC().Format(time.RFC3339))
		b.WriteString("\n")
	}
	b.WriteString("---\n\n")

	if title != "" {
		b.WriteString("# ")
		b.WriteString(title)
		b.WriteString("\n\n")
	}
	b.WriteString(content)
	if url != "" {
		b.WriteString("\n\n---\n[Source](")
		b.WriteString(url)
		b.WriteString(")")
	}
	return b.String()
}

// quoteYAML wraps a string in double-quotes with internal quotes
// doubled. Good enough for safe front-matter in v0.7.25; full YAML
// escaping is overkill for the connector payload.
func quoteYAML(s string) string {
	if !strings.ContainsAny(s, ":#\n\"") {
		return s
	}
	return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
}
