package kg

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// KGGraphNode is the public-facing shape returned by the graph API.
// Decoupled from KGEntity so the visualization layer can change
// without touching the data model.
type KGGraphNode struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	SupertagID string  `json:"supertag_id,omitempty"`
	Supertag   string  `json:"supertag,omitempty"`
	KBID       string  `json:"kb_id"`
	Occurrence int     `json:"occurrence"`
	NodeSize   float64 `json:"node_size"`
	Color      string  `json:"color"`
}

// KGGraphEdge is the public-facing shape for relations.
type KGGraphEdge struct {
	ID         string  `json:"id"`
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
	Evidence   string  `json:"evidence,omitempty"`
}

// KGGraph is the assembled graph for one KB.
type KGGraph struct {
	KBID        string         `json:"kb_id"`
	SupertagID  string         `json:"supertag_id,omitempty"`
	Nodes       []*KGGraphNode `json:"nodes"`
	Edges       []*KGGraphEdge `json:"edges"`
	GeneratedAt int64          `json:"generated_at"`
}

// KGGraphService composes the KG repository into a single graph query.
type KGGraphService struct {
	repo interfaces.KGRepository
	now  func() time.Time
}

// NewKGGraphService constructs a KGGraphService.
func NewKGGraphService(repo interfaces.KGRepository) *KGGraphService {
	return &KGGraphService{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

// SetNow lets tests freeze time.
func (s *KGGraphService) SetNow(now func() time.Time) { s.now = now }

// BuildGraph returns nodes + edges for a KB.
func (s *KGGraphService) BuildGraph(ctx context.Context, tenantID uint64, kbID, supertagID string, limit int) (*KGGraph, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	g := &KGGraph{KBID: kbID, SupertagID: supertagID}

	var entities []*types.KGEntity
	var err error
	if supertagID != "" {
		entities, err = s.repo.ListEntitiesBySupertag(ctx, tenantID, supertagID, limit)
	} else {
		entities, err = s.listAllEntitiesInKB(ctx, tenantID, kbID, limit)
	}
	if err != nil {
		return nil, err
	}

	idSeen := make(map[string]struct{}, len(entities))
	for _, e := range entities {
		if _, dup := idSeen[e.ID]; dup {
			continue
		}
		idSeen[e.ID] = struct{}{}
		var stid string
		if e.SupertagID != nil {
			stid = *e.SupertagID
		}
		g.Nodes = append(g.Nodes, &KGGraphNode{
			ID:         e.ID,
			Name:       e.Name,
			SupertagID: stid,
			KBID:       e.KBID,
			Occurrence: e.Occurrence,
			NodeSize:   entitySize(e.Occurrence),
			Color:      supertagColor(stid),
		})
	}

	edgesByID := make(map[string]*KGGraphEdge)
	for _, e := range entities {
		rels, err := s.repo.ListRelationsByEntity(ctx, tenantID, e.ID, 200)
		if err != nil {
			continue
		}
		for _, r := range rels {
			if _, ok := idSeen[r.SrcEntityID]; !ok {
				continue
			}
			if _, ok := idSeen[r.DstEntityID]; !ok {
				continue
			}
			if existing, ok := edgesByID[r.ID]; ok {
				if r.Confidence > existing.Confidence {
					existing.Confidence = r.Confidence
				}
				continue
			}
			edgesByID[r.ID] = &KGGraphEdge{
				ID:         r.ID,
				Source:     r.SrcEntityID,
				Target:     r.DstEntityID,
				Type:       r.Relation,
				Confidence: r.Confidence,
			}
		}
	}
	for _, edge := range edgesByID {
		g.Edges = append(g.Edges, edge)
	}

	sort.SliceStable(g.Nodes, func(i, j int) bool {
		return g.Nodes[i].ID < g.Nodes[j].ID
	})
	sort.SliceStable(g.Edges, func(i, j int) bool {
		return g.Edges[i].ID < g.Edges[j].ID
	})
	g.GeneratedAt = s.now().Unix()
	return g, nil
}

func (s *KGGraphService) listAllEntitiesInKB(ctx context.Context, tenantID uint64, kbID string, limit int) ([]*types.KGEntity, error) {
	supertags, err := s.repo.ListSupertagsByKB(ctx, tenantID, kbID)
	if err != nil {
		return nil, err
	}
	if len(supertags) == 0 {
		return []*types.KGEntity{}, nil
	}
	perTag := limit / len(supertags)
	if perTag < 1 {
		perTag = 1
	}
	out := []*types.KGEntity{}
	for _, st := range supertags {
		es, err := s.repo.ListEntitiesBySupertag(ctx, tenantID, st.ID, perTag)
		if err != nil {
			continue
		}
		out = append(out, es...)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func entitySize(occurrence int) float64 {
	if occurrence <= 0 {
		return 4
	}
	v := float64(occurrence)
	if v > 1000 {
		v = 1000
	}
	return 4 + 20*math.Log10(1+v)/3
}

func supertagColor(supertagID string) string {
	if supertagID == "" {
		return "#888888"
	}
	h := uint32(2166136261)
	for i := 0; i < len(supertagID); i++ {
		h ^= uint32(supertagID[i])
		h *= 16777619
	}
	r := 60 + int((h>>16)&0xFF)%196
	g := 60 + int((h>>8)&0xFF)%196
	b := 60 + int(h&0xFF)%196
	return "#" + toHex2(r) + toHex2(g) + toHex2(b)
}

func toHex2(v int) string {
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	const hex = "0123456789abcdef"
	return string([]byte{hex[v>>4], hex[v&0xF]})
}
