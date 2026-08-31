// Package workflow implements the Build #37 AI Workflow Builder.
package workflow

import (
	"errors"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
)

// Sentinel errors surfaced by workflow validation.
var (
	ErrEmptyWorkflow  = errors.New("workflow: no nodes")
	ErrCyclicWorkflow = errors.New("workflow: cyclic dependency")
	ErrUnknownNode    = errors.New("workflow: edge references unknown node")
	ErrNoTrigger      = errors.New("workflow: at least one trigger node is required")
)

// ValidateDAG enforces structural invariants on a workflow definition:
//   1. At least one trigger node is present.
//   2. Node IDs are unique.
//   3. Edge endpoints reference existing nodes.
//   4. The graph is acyclic (cycles would cause infinite loops at run time).
//
// The algorithm is a depth-first search that colour-marks each node
// white / gray / black — gray-on-gray is a back-edge and therefore a cycle.
func ValidateDAG(w *types.Workflow) error {
	if w == nil || len(w.Nodes) == 0 {
		return ErrEmptyWorkflow
	}
	idSeen := make(map[string]bool, len(w.Nodes))
	for _, n := range w.Nodes {
		if n.ID == "" {
			return errors.New("workflow: node id is required")
		}
		if idSeen[n.ID] {
			return fmt.Errorf("workflow: duplicate node id %q", n.ID)
		}
		idSeen[n.ID] = true
	}
	// Trigger requirement.
	hasTrigger := false
	for _, n := range w.Nodes {
		switch n.Type {
		case types.WorkflowTriggerManual,
			types.WorkflowTriggerScheduled,
			types.WorkflowTriggerEvent,
			types.WorkflowTriggerWebhook,
			types.WorkflowTriggerFormSubmit:
			hasTrigger = true
		}
	}
	if !hasTrigger {
		return ErrNoTrigger
	}
	// Edge endpoints must reference existing nodes.
	for _, e := range w.Edges {
		if !idSeen[e.SrcNodeID] {
			return fmt.Errorf("%w: edge %s -> %s (src missing)", ErrUnknownNode, e.SrcNodeID, e.DstNodeID)
		}
		if !idSeen[e.DstNodeID] {
			return fmt.Errorf("%w: edge %s -> %s (dst missing)", ErrUnknownNode, e.SrcNodeID, e.DstNodeID)
		}
	}
	// Cycle detection via DFS.
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(w.Nodes))
	for k := range idSeen {
		color[k] = white
	}
	outEdges := make(map[string][]string, len(w.Nodes))
	for _, e := range w.Edges {
		outEdges[e.SrcNodeID] = append(outEdges[e.SrcNodeID], e.DstNodeID)
	}
	var visit func(id string) error
	visit = func(id string) error {
		switch color[id] {
		case gray:
			return fmt.Errorf("%w: through %s", ErrCyclicWorkflow, id)
		case black:
			return nil
		}
		color[id] = gray
		for _, next := range outEdges[id] {
			if err := visit(next); err != nil {
				return err
			}
		}
		color[id] = black
		return nil
	}
	for id := range idSeen {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

// TopologicalSort returns node IDs in a valid execution order. Nodes with
// no incoming edges (i.e. trigger nodes) come first; downstream nodes
// appear after their last predecessor.
func TopologicalSort(w *types.Workflow) ([]string, error) {
	if err := ValidateDAG(w); err != nil {
		return nil, err
	}
	inDeg := make(map[string]int, len(w.Nodes))
	outEdges := make(map[string][]string, len(w.Nodes))
	for _, n := range w.Nodes {
		inDeg[n.ID] = 0
	}
	for _, e := range w.Edges {
		inDeg[e.DstNodeID]++
		outEdges[e.SrcNodeID] = append(outEdges[e.SrcNodeID], e.DstNodeID)
	}
	queue := make([]string, 0, len(w.Nodes))
	for id, d := range inDeg {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	out := make([]string, 0, len(w.Nodes))
	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]
		out = append(out, head)
		for _, next := range outEdges[head] {
			inDeg[next]--
			if inDeg[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	return out, nil
}
