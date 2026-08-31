package kg

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
)

// REPipeline (Relation Extraction) infers typed edges between entities
// identified by NER. It produces KGRelationDrafts that downstream code can
// persist as KGEntityRelations.
type REPipeline struct {
	llm LLMClient
}

// NewREPipeline constructs an RE pipeline.
func NewREPipeline(llm LLMClient) *REPipeline { return &REPipeline{llm: llm} }

// Extract runs relation extraction on the supplied entity list. The text
// is supplied for context; the model is expected to produce only edges
// supported by the text.
func (p *REPipeline) Extract(ctx context.Context, documentID, text string, entities []types. KGEntityDraft) ([]types.KGRelationDraft, error) {
	if p.llm == nil || len(entities) < 2 {
		return p.extractHeuristic(entities), nil
	}
	entityJSON, _ := json.Marshal(entities)
	system := "You are a precise relation extractor. Output strict JSON only."
	user := fmt.Sprintf(`Given the entities and the passage, return a JSON array of relations: [{"src":"<entity name>","dst":"<entity name>","relation":"works_at|manages|located_in|part_of|related_to","confidence":0.0-1.0}].
Entities: %s
Passage: %s`, entityJSON, text)
	out, err := p.llm.Complete(ctx, system, user)
	if err != nil {
		return p.extractHeuristic(entities), nil
	}
	out = strings.TrimSpace(out)
	out = strings.TrimPrefix(out, "```json")
	out = strings.TrimPrefix(out, "```")
	out = strings.TrimSuffix(out, "```")
	type relDTO struct {
		Src        string  `json:"src"`
		Dst        string  `json:"dst"`
		Relation   string  `json:"relation"`
		Confidence float64 `json:"confidence"`
	}
	var raws []relDTO
	if err := json.Unmarshal([]byte(out), &raws); err != nil {
		return p.extractHeuristic(entities), nil
	}
	byName := make(map[string]types. KGEntityDraft, len(entities))
	for _, e := range entities {
		byName[e.Name] = e
	}
	var out2 []types.KGRelationDraft
	for _, r := range raws {
		src, ok1 := byName[r.Src]
		dst, ok2 := byName[r.Dst]
		if !ok1 || !ok2 {
			continue
		}
		if r.Confidence == 0 {
			r.Confidence = 0.5
		}
		out2 = append(out2, types.KGRelationDraft{
			SrcTmpID:   src.TmpID,
			DstTmpID:   dst.TmpID,
			Relation:   r.Relation,
			Confidence: r.Confidence,
		})
	}
	return out2, nil
}

// extractHeuristic returns a single "co_occurs" edge per pair as a
// deterministic fallback when the LLM is unavailable.
func (p *REPipeline) extractHeuristic(entities []types. KGEntityDraft) []types.KGRelationDraft {
	if len(entities) < 2 {
		return nil
	}
	out := make([]types.KGRelationDraft, 0, len(entities)-1)
	for i := 1; i < len(entities); i++ {
		out = append(out, types.KGRelationDraft{
			SrcTmpID:   entities[i-1].TmpID,
			DstTmpID:   entities[i].TmpID,
			Relation:   "co_occurs",
			Confidence: 0.2,
		})
	}
	return out
}

// PersistDrafts converts the NER + RE drafts into persistent KGEntity /
// KGEntityRelation rows, deduplicating by name (existing entities get
// their occurrence counter bumped).
func (p *REPipeline) PersistDrafts(ctx context.Context, svc *KGSupertagService, tenantID uint64, kbID, documentID string, result *types.KGExtractionResult) error {
	nameToID := make(map[string]string)
	for _, d := range result.Entities {
		existing, err := svc.repo.FindEntitiesByName(ctx, tenantID, kbID, d.Name)
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			e := existing[0]
			nameToID[d.Name] = e.ID
			_ = svc.repo.BumpEntityOccurrence(ctx, e.ID)
			continue
		}
		e := &types. KGEntity{
			TenantID:     tenantID,
			KBID:         kbID,
			Name:         d.Name,
			Properties:   json.RawMessage("{}"),
			FirstSeenDoc: &documentID,
			LastSeenDoc:  &documentID,
			Occurrence:   1,
			TrustScore:   d.Confidence,
		}
		e.ID = uuid.NewString()
		now := svc.now()
		e.CreatedAt = now
		e.UpdatedAt = now
		if err := svc.repo.CreateEntity(ctx, e); err != nil {
			return err
		}
		nameToID[d.Name] = e.ID
	}
	// Relations are mapped back to TmpID -> Name -> persistent ID.
	tmpToName := make(map[string]string, len(result.Entities))
	for _, d := range result.Entities {
		tmpToName[d.TmpID] = d.Name
	}
	for _, r := range result.Relations {
		srcName, ok1 := tmpToName[r.SrcTmpID]
		dstName, ok2 := tmpToName[r.DstTmpID]
		if !ok1 || !ok2 {
			continue
		}
		srcID := nameToID[srcName]
		dstID := nameToID[dstName]
		rel := &types.KGEntityRelation{
			SrcEntityID:  srcID,
			DstEntityID:  dstID,
			Relation:     r.Relation,
			Weight:       r.Confidence,
			EvidenceDocs: json.RawMessage("[]"),
			Confidence:   r.Confidence,
		}
		rel.ID = uuid.NewString()
		rel.CreatedAt = svc.now()
		_ = svc.repo.CreateRelation(ctx, rel)
	}
	return nil
}
