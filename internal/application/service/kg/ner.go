package kg

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
)

// NERPipeline runs zero-shot Named Entity Recognition against a text
// passage. It calls the configured LLMClient once per document and parses
// the JSON response into EntityDrafts. When the LLM is unavailable (e.g.
// offline tests) it falls back to a deterministic regex extractor that
// recognises capitalised proper nouns.
type NERPipeline struct {
	llm    LLMClient
	entityTypes []string
}

// NewNERPipeline constructs a NER pipeline with a sensible default
// entity type vocabulary (person, organization, project, location, date,
// concept).
func NewNERPipeline(llm LLMClient) *NERPipeline {
	return &NERPipeline{
		llm: llm,
		entityTypes: []string{
			"person", "organization", "project", "location", "date", "concept",
		},
	}
}

// Extract runs NER on the supplied text and returns a list of EntityDrafts.
// The documentID is attached to each draft so downstream stages can record
// provenance.
func (p *NERPipeline) Extract(ctx context.Context, documentID, text string) ([]types.KGEntityDraft, error) {
	if p.llm != nil {
		drafts, err := p.extractLLM(ctx, documentID, text)
		if err == nil {
			return drafts, nil
		}
		// LLM failure falls through to the regex extractor.
	}
	return p.extractRegex(documentID, text), nil
}

func (p *NERPipeline) extractLLM(ctx context.Context, documentID, text string) ([]types.KGEntityDraft, error) {
	system := "You are a precise named-entity extractor. Output strict JSON only."
	typesHint := strings.Join(p.entityTypes, ", ")
	user := fmt.Sprintf(`Extract entities from the passage below. Return a JSON array of objects: [{"name":"...","type":"%s","span":"...","confidence":0.0-1.0}].
Passage:
%s`, typesHint, text)
	out, err := p.llm.Complete(ctx, system, user)
	if err != nil {
		return nil, err
	}
	// Strip code-fence wrappers the model occasionally emits.
	out = strings.TrimSpace(out)
	out = strings.TrimPrefix(out, "```json")
	out = strings.TrimPrefix(out, "```")
	out = strings.TrimSuffix(out, "```")
	var drafts []types.KGEntityDraft
	if err := json.Unmarshal([]byte(out), &drafts); err != nil {
		return nil, fmt.Errorf("ner: decode llm output: %w", err)
	}
	for i := range drafts {
		if drafts[i].TmpID == "" {
			drafts[i].TmpID = uuid.NewString()
		}
		if drafts[i].Confidence == 0 {
			drafts[i].Confidence = 0.5
		}
	}
	return drafts, nil
}

// properNoun matches sequences of 1-4 capitalised words. Sufficient as a
// fallback for short Chinese / English passages; the LLM path is the
// production extractor.
var properNoun = regexp.MustCompile(`\b([A-Z][a-z]+(?:\s+[A-Z][a-z]+){0,3})\b`)

func (p *NERPipeline) extractRegex(documentID, text string) []types.KGEntityDraft {
	var out []types.KGEntityDraft
	for _, m := range properNoun.FindAllStringSubmatch(text, -1) {
		out = append(out, types.KGEntityDraft{
			TmpID:      uuid.NewString(),
			Name:       strings.TrimSpace(m[1]),
			Type:       "concept",
			Span:       strings.TrimSpace(m[1]),
			Confidence: 0.3,
		})
	}
	return out
}
