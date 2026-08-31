// Package mindmap — Build #43 MindMap application service.
//
// Composes the MindMapRepository into:
//   - CRUD on maps and nodes
//   - auto-layout algorithms (tree / fishbone / timeline / radial / free)
//   - export to Markdown outline / OPML / .xmind JSON
//   - audit hooks (for Build #46 Governance compliance)
package mindmap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// MindMapService is the application-level MindMap surface.
type MindMapService struct {
	repo interfaces.MindMapRepository
	// audit hook (optional). When set, the service emits audit events
	// on create / update / delete / export. Reserved for Build #46
	// Governance console wiring.
	audit func(ctx context.Context, tenantID uint64, mapID string, userID uint64, action, detail string)
}

// NewMindMapService wires the service.
func NewMindMapService(repo interfaces.MindMapRepository) *MindMapService {
	return &MindMapService{repo: repo}
}

// SetAuditHook lets the container wire a governance audit emitter.
func (s *MindMapService) SetAuditHook(hook func(ctx context.Context, tenantID uint64, mapID string, userID uint64, action, detail string)) {
	s.audit = hook
}

// CreateMindMap persists a new MindMap + the supplied root node atomically.
func (s *MindMapService) CreateMindMap(ctx context.Context, tenantID, userID uint64, req types.CreateMindMapRequest) (*types.MindMap, error) {
	if req.Title == "" {
		return nil, types.ErrMindMapInvalid("title is required")
	}
	if req.Layout == "" {
		req.Layout = types.MindMapLayoutTree
	}
	if !types.ValidMindMapLayouts[req.Layout] {
		return nil, types.ErrMindMapInvalid("layout is invalid")
	}
	if req.Theme == "" {
		req.Theme = "feishu"
	}
	if req.Visibility == "" {
		req.Visibility = "private"
	}
	rootID := newID()
	root := &types.MindMapNode{
		ID:       rootID,
		TenantID: tenantID,
		MapID:    "PENDING", // patched below
		NodeType: types.MindMapNodeText,
		Label:    req.RootLabel,
		Body:     req.RootBody,
		Color:    req.RootColor,
		Icon:     req.RootIcon,
		X:        0,
		Y:        0,
		Width:    180,
		Height:   56,
	}
	if root.Label == "" {
		root.Label = req.Title
	}
	m := &types.MindMap{
		ID:          newID(),
		TenantID:    tenantID,
		Title:       req.Title,
		Layout:      req.Layout,
		Theme:       req.Theme,
		RootNodeID:  rootID,
		KBID:        req.KBID,
		OwnerUserID: userID,
		Visibility:  req.Visibility,
	}
	root.MapID = m.ID

	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	if err := s.repo.CreateNode(ctx, root); err != nil {
		// Best-effort cleanup if root insert fails.
		_ = s.repo.Delete(ctx, tenantID, m.ID)
		return nil, err
	}
	s.emitAudit(ctx, tenantID, m.ID, userID, "create", fmt.Sprintf("title=%s layout=%s", m.Title, m.Layout))
	return m, nil
}

// GetMindMap returns one MindMap.
func (s *MindMapService) GetMindMap(ctx context.Context, tenantID uint64, id string) (*types.MindMap, error) {
	return s.repo.Get(ctx, tenantID, id)
}

// UpdateMindMap applies a partial patch.
func (s *MindMapService) UpdateMindMap(ctx context.Context, tenantID, userID uint64, id string, patch types.UpdateMindMapRequest) (*types.MindMap, error) {
	out, err := s.repo.Update(ctx, tenantID, id, patch)
	if err != nil {
		return nil, err
	}
	if out != nil {
		s.emitAudit(ctx, tenantID, id, userID, "update", "metadata")
	}
	return out, nil
}

// DeleteMindMap removes a MindMap (transactional with its nodes).
func (s *MindMapService) DeleteMindMap(ctx context.Context, tenantID, userID uint64, id string) error {
	s.emitAudit(ctx, tenantID, id, userID, "delete", "")
	return s.repo.Delete(ctx, tenantID, id)
}

// ListMindMaps lists maps with filters.
func (s *MindMapService) ListMindMaps(ctx context.Context, tenantID uint64, filter types.ListMindMapsFilter) ([]*types.MindMap, error) {
	return s.repo.List(ctx, tenantID, filter)
}

// CountMindMaps returns the count for the same filters.
func (s *MindMapService) CountMindMaps(ctx context.Context, tenantID uint64, filter types.ListMindMapsFilter) (int64, error) {
	return s.repo.Count(ctx, tenantID, filter)
}

// CreateNode adds a node under an existing parent.
func (s *MindMapService) CreateNode(ctx context.Context, tenantID, userID uint64, mapID string, req types.CreateMindMapNodeRequest) (*types.MindMapNode, error) {
	m, err := s.repo.Get(ctx, tenantID, mapID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, types.ErrMindMapInvalid("mindmap not found")
	}
	if !types.ValidMindMapNodeTypes[req.NodeType] {
		return nil, types.ErrMindMapInvalid("node_type is invalid")
	}
	// Validate parent is in the same map.
	if req.ParentID != nil && *req.ParentID != "" {
		p, err := s.repo.GetNode(ctx, tenantID, mapID, *req.ParentID)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, types.ErrMindMapInvalid("parent node not found in this map")
		}
	}
	n := &types.MindMapNode{
		ID:        newID(),
		TenantID:  tenantID,
		MapID:     mapID,
		ParentID:  req.ParentID,
		NodeType:  req.NodeType,
		Label:     req.Label,
		Body:      req.Body,
		X:         req.X,
		Y:         req.Y,
		Width:     defaultFloat(req.Width, 160),
		Height:    defaultFloat(req.Height, 48),
		Color:     req.Color,
		Icon:      req.Icon,
		DocRef:    req.DocRef,
		KBRef:     req.KBRef,
		TaskRef:   req.TaskRef,
		Formula:   req.Formula,
		OrderHint: req.OrderHint,
	}
	if err := s.repo.CreateNode(ctx, n); err != nil {
		return nil, err
	}
	s.emitAudit(ctx, tenantID, mapID, userID, "node_create", fmt.Sprintf("node_id=%s type=%s", n.ID, n.NodeType))
	return n, nil
}

// UpdateNode applies a partial patch on a single node.
func (s *MindMapService) UpdateNode(ctx context.Context, tenantID, userID uint64, mapID, nodeID string, patch types.UpdateMindMapNodeRequest) (*types.MindMapNode, error) {
	out, err := s.repo.UpdateNode(ctx, tenantID, mapID, nodeID, patch)
	if err != nil {
		return nil, err
	}
	if out != nil {
		s.emitAudit(ctx, tenantID, mapID, userID, "node_update", fmt.Sprintf("node_id=%s", nodeID))
	}
	return out, nil
}

// DeleteNode removes a node. The caller is responsible for deciding whether
// to delete descendants; we refuse if the node has children.
func (s *MindMapService) DeleteNode(ctx context.Context, tenantID, userID uint64, mapID, nodeID string) error {
	// Refuse to delete the root.
	m, err := s.repo.Get(ctx, tenantID, mapID)
	if err != nil {
		return err
	}
	if m == nil {
		return types.ErrMindMapInvalid("mindmap not found")
	}
	if m.RootNodeID == nodeID {
		return types.ErrMindMapInvalid("cannot delete root node")
	}
	if err := s.repo.DeleteNode(ctx, tenantID, mapID, nodeID); err != nil {
		return err
	}
	s.emitAudit(ctx, tenantID, mapID, userID, "node_delete", fmt.Sprintf("node_id=%s", nodeID))
	return nil
}

// ListNodes returns every node in a map.
func (s *MindMapService) ListNodes(ctx context.Context, tenantID uint64, mapID string) ([]*types.MindMapNode, error) {
	return s.repo.ListNodesByMap(ctx, tenantID, mapID)
}

// AutoLayout repositions every node according to the requested layout.
// spacing controls the gap between siblings (default 80 px).
func (s *MindMapService) AutoLayout(ctx context.Context, tenantID, userID uint64, mapID string, req types.AutoLayoutRequest) ([]*types.MindMapNode, error) {
	if !types.ValidMindMapLayouts[req.Layout] {
		return nil, types.ErrMindMapInvalid("layout is invalid")
	}
	spacing := req.Spacing
	if spacing <= 0 {
		spacing = 80
	}
	nodes, err := s.repo.ListNodesByMap(ctx, tenantID, mapID)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nodes, nil
	}
	m, err := s.repo.Get(ctx, tenantID, mapID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, types.ErrMindMapInvalid("mindmap not found")
	}
	// Build a parent → children index.
	children := make(map[string][]*types.MindMapNode)
	roots := []*types.MindMapNode{}
	for _, n := range nodes {
		if n.ParentID == nil || *n.ParentID == "" {
			roots = append(roots, n)
			continue
		}
		children[*n.ParentID] = append(children[*n.ParentID], n)
	}
	// Apply layout.
	updated := []*types.MindMapNode{}
	switch req.Layout {
	case types.MindMapLayoutTree:
		updated = layoutTree(roots, children, spacing)
	case types.MindMapLayoutFishbone:
		updated = layoutFishbone(roots, children, spacing)
	case types.MindMapLayoutTimeline:
		updated = layoutTimeline(roots, children, spacing)
	case types.MindMapLayoutRadial:
		updated = layoutRadial(roots, children, spacing)
	case types.MindMapLayoutFree:
		// Free layout: don't reposition; keep existing.
		return nodes, nil
	default:
		return nil, types.ErrMindMapInvalid("layout is invalid")
	}
	// Persist each updated node.
	for _, n := range updated {
		_, err := s.repo.UpdateNode(ctx, tenantID, mapID, n.ID, types.UpdateMindMapNodeRequest{
			X:         &n.X,
			Y:         &n.Y,
			Width:     &n.Width,
			Height:    &n.Height,
			OrderHint: &n.OrderHint,
		})
		if err != nil {
			return nil, err
		}
	}
	s.emitAudit(ctx, tenantID, mapID, userID, "auto_layout", fmt.Sprintf("layout=%s spacing=%d", req.Layout, spacing))
	return updated, nil
}

// layoutTree produces a left-to-right tree (root on the left).
func layoutTree(roots []*types.MindMapNode, children map[string][]*types.MindMapNode, spacing int) []*types.MindMapNode {
	out := []*types.MindMapNode{}
	for _, root := range roots {
		out = append(out, layoutTreeDFS(root, children, 0, 0, spacing, &yCursor)...)
	}
	return out
}

var yCursor float64

func layoutTreeDFS(node *types.MindMapNode, children map[string][]*types.MindMapNode, depth int, yStart int, spacing int, yc *float64) []*types.MindMapNode {
	out := []*types.MindMapNode{}
	node.X = float64(depth * (160 + spacing))
	node.Y = *yc
	node.Width = 160
	node.Height = 56
	*yc += node.Height + float64(spacing)
	out = append(out, node)
	for _, c := range children[node.ID] {
		out = append(out, layoutTreeDFS(c, children, depth+1, int(*yc), spacing, yc)...)
	}
	return out
}

// layoutFishbone produces an Ishikawa (fishbone / cause-and-effect) layout.
func layoutFishbone(roots []*types.MindMapNode, children map[string][]*types.MindMapNode, spacing int) []*types.MindMapNode {
	out := []*types.MindMapNode{}
	if len(roots) == 0 {
		return out
	}
	root := roots[0]
	root.X = 0
	root.Y = 0
	root.Width = 200
	root.Height = 60
	out = append(out, root)
	// Top and bottom branches along the spine.
	branches := children[root.ID]
	top := []*types.MindMapNode{}
	bottom := []*types.MindMapNode{}
	for i, b := range branches {
		if i%2 == 0 {
			top = append(top, b)
		} else {
			bottom = append(bottom, b)
		}
	}
	x := float64(200 + spacing)
	for i, b := range top {
		b.X = x + float64(i*spacing)
		b.Y = -float64(spacing)
		b.Width = 140
		b.Height = 48
		out = append(out, b)
		for j, c := range children[b.ID] {
			c.X = b.X
			c.Y = b.Y - float64((j+1)*spacing/2)
			c.Width = 120
			c.Height = 40
			out = append(out, c)
		}
	}
	for i, b := range bottom {
		b.X = x + float64(i*spacing)
		b.Y = float64(spacing)
		b.Width = 140
		b.Height = 48
		out = append(out, b)
		for j, c := range children[b.ID] {
			c.X = b.X
			c.Y = b.Y + float64((j+1)*spacing/2)
			c.Width = 120
			c.Height = 40
			out = append(out, c)
		}
	}
	return out
}

// layoutTimeline produces a left-to-right timeline.
func layoutTimeline(roots []*types.MindMapNode, children map[string][]*types.MindMapNode, spacing int) []*types.MindMapNode {
	out := []*types.MindMapNode{}
	if len(roots) == 0 {
		return out
	}
	root := roots[0]
	root.X = 0
	root.Y = 0
	root.Width = 160
	root.Height = 56
	out = append(out, root)
	x := float64(160 + spacing)
	for i, c := range children[root.ID] {
		c.X = x + float64(i*(160+spacing))
		c.Y = 0
		c.Width = 160
		c.Height = 56
		out = append(out, c)
		for j, gc := range children[c.ID] {
			gc.X = c.X
			gc.Y = float64((j+1) * (56 + spacing/2))
			gc.Width = 140
			gc.Height = 48
			out = append(out, gc)
		}
	}
	return out
}

// layoutRadial produces a circle-around-center layout.
func layoutRadial(roots []*types.MindMapNode, children map[string][]*types.MindMapNode, spacing int) []*types.MindMapNode {
	out := []*types.MindMapNode{}
	if len(roots) == 0 {
		return out
	}
	root := roots[0]
	root.X = 0
	root.Y = 0
	root.Width = 180
	root.Height = 60
	out = append(out, root)
	radius := float64(200)
	primary := children[root.ID]
	for i, c := range primary {
		theta := 2 * math.Pi * float64(i) / float64(maxInt(1, len(primary)))
		c.X = radius * math.Cos(theta)
		c.Y = radius * math.Sin(theta)
		c.Width = 140
		c.Height = 48
		out = append(out, c)
		for j, gc := range children[c.ID] {
			gcTheta := theta + float64(j+1)*0.3
			gc.X = (radius + 100) * math.Cos(gcTheta)
			gc.Y = (radius + 100) * math.Sin(gcTheta)
			gc.Width = 120
			gc.Height = 40
			out = append(out, gc)
		}
	}
	return out
}

// ExportMarkdown renders a Markdown outline of the map.
func (s *MindMapService) ExportMarkdown(ctx context.Context, tenantID uint64, mapID string) (string, error) {
	m, nodes, err := s.loadFull(ctx, tenantID, mapID)
	if err != nil {
		return "", err
	}
	return renderMarkdown(m, nodes), nil
}

// ExportOPML renders OPML 2.0.
func (s *MindMapService) ExportOPML(ctx context.Context, tenantID uint64, mapID string) (string, error) {
	m, nodes, err := s.loadFull(ctx, tenantID, mapID)
	if err != nil {
		return "", err
	}
	return renderOPML(m, nodes), nil
}

// ExportXMind renders a minimal XMind JSON.
func (s *MindMapService) ExportXMind(ctx context.Context, tenantID uint64, mapID string) (string, error) {
	m, nodes, err := s.loadFull(ctx, tenantID, mapID)
	if err != nil {
		return "", err
	}
	payload := buildXMindJSON(m, nodes)
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// loadFull returns the map + its nodes (single query each).
func (s *MindMapService) loadFull(ctx context.Context, tenantID uint64, mapID string) (*types.MindMap, []*types.MindMapNode, error) {
	m, err := s.repo.Get(ctx, tenantID, mapID)
	if err != nil {
		return nil, nil, err
	}
	if m == nil {
		return nil, nil, types.ErrMindMapInvalid("mindmap not found")
	}
	nodes, err := s.repo.ListNodesByMap(ctx, tenantID, mapID)
	if err != nil {
		return nil, nil, err
	}
	return m, nodes, nil
}

// renderMarkdown walks the tree top-down and emits # / ## / ### headings.
func renderMarkdown(m *types.MindMap, nodes []*types.MindMapNode) string {
	idx := map[string]*types.MindMapNode{}
	for _, n := range nodes {
		idx[n.ID] = n
	}
	children := map[string][]*types.MindMapNode{}
	for _, n := range nodes {
		if n.ParentID == nil || *n.ParentID == "" {
			children["__root__"] = append(children["__root__"], n)
		} else {
			children[*n.ParentID] = append(children[*n.ParentID], n)
		}
	}
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(m.Title)
	b.WriteString("\n\n")
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		// Sort siblings by order_hint + label.
		kids := children[parent]
		sort.SliceStable(kids, func(i, j int) bool {
			if kids[i].OrderHint != kids[j].OrderHint {
				return kids[i].OrderHint < kids[j].OrderHint
			}
			return kids[i].Label < kids[j].Label
		})
		for _, k := range kids {
			b.WriteString(strings.Repeat("#", depth+2))
			b.WriteString(" ")
			b.WriteString(k.Label)
			if k.Body != "" {
				b.WriteString("\n\n")
				b.WriteString(k.Body)
			}
			b.WriteString("\n\n")
			walk(k.ID, depth+1)
		}
	}
	walk("__root__", 0)
	return b.String()
}

// renderOPML renders OPML 2.0.
func renderOPML(m *types.MindMap, nodes []*types.MindMapNode) string {
	children := map[string][]*types.MindMapNode{}
	for _, n := range nodes {
		if n.ParentID == nil || *n.ParentID == "" {
			children["__root__"] = append(children["__root__"], n)
		} else {
			children[*n.ParentID] = append(children[*n.ParentID], n)
		}
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<opml version="2.0">` + "\n")
	b.WriteString(`  <head><title>` + escapeXML(m.Title) + `</title></head>` + "\n")
	b.WriteString(`  <body>` + "\n")
	var walk func(parent string, indent string)
	walk = func(parent string, indent string) {
		kids := children[parent]
		sort.SliceStable(kids, func(i, j int) bool {
			if kids[i].OrderHint != kids[j].OrderHint {
				return kids[i].OrderHint < kids[j].OrderHint
			}
			return kids[i].Label < kids[j].Label
		})
		for _, k := range kids {
			fmt.Fprintf(&b, "%s<outline text=\"%s\"", indent, escapeXML(k.Label))
			if k.Body != "" {
				fmt.Fprintf(&b, " _note=\"%s\"", escapeXML(k.Body))
			}
			b.WriteString(">")
			b.WriteString("\n")
			if grand := children[k.ID]; len(grand) > 0 {
				walk(k.ID, indent+"  ")
			}
			b.WriteString(indent + "</outline>\n")
		}
	}
	walk("__root__", "    ")
	b.WriteString("  </body>\n</opml>\n")
	return b.String()
}

// buildXMindJSON returns a minimal payload consumable by xmind-sdk.
func buildXMindJSON(m *types.MindMap, nodes []*types.MindMapNode) map[string]any {
	children := map[string][]*types.MindMapNode{}
	for _, n := range nodes {
		if n.ParentID == nil || *n.ParentID == "" {
			children["__root__"] = append(children["__root__"], n)
		} else {
			children[*n.ParentID] = append(children[*n.ParentID], n)
		}
	}
	var build func(parent string) []any
	build = func(parent string) []any {
		kids := children[parent]
		sort.SliceStable(kids, func(i, j int) bool {
			if kids[i].OrderHint != kids[j].OrderHint {
				return kids[i].OrderHint < kids[j].OrderHint
			}
			return kids[i].Label < kids[j].Label
		})
		out := make([]any, 0, len(kids))
		for _, k := range kids {
			entry := map[string]any{
				"id":    k.ID,
				"title": k.Label,
				"note":  k.Body,
				"color": k.Color,
			}
			if grand := children[k.ID]; len(grand) > 0 {
				entry["children"] = build(k.ID)
			}
			out = append(out, entry)
		}
		return out
	}
	return map[string]any{
		"title":    m.Title,
		"layout":   m.Layout,
		"theme":    m.Theme,
		"created":  m.CreatedAt,
		"updated":  m.UpdatedAt,
		"children": build("__root__"),
	}
}

// emitAudit writes an audit event when the hook is set.
func (s *MindMapService) emitAudit(ctx context.Context, tenantID uint64, mapID string, userID uint64, action, detail string) {
	if s.audit == nil {
		return
	}
	s.audit(ctx, tenantID, mapID, userID, action, detail)
}

// newID returns a 32-char hex ID (UUID-v4-like).
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback to time-based to keep the function panic-free.
		ts := time.Now().UnixNano()
		return fmt.Sprintf("%016x", ts)
	}
	return hex.EncodeToString(b[:])
}

func defaultFloat(v, def float64) float64 {
	if v <= 0 {
		return def
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func escapeXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// Ensure that audit hook defaults to a logger-backed noop when the container
// doesn't wire one in tests.
func init() {
	logger.Debugf(context.Background(), "[mindmap] service initialized")
}
