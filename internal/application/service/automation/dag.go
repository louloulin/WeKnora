package automation

import (
	"errors"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
)

// ErrCyclicDependency is returned by ValidateDAG when an automation
// has a cycle in its step graph.
var ErrCyclicDependency = errors.New("automation: cyclic dependency")

// ErrUnknownStep is returned when a step's NextIDs reference a step
// id that does not exist in the automation.
var ErrUnknownStep = errors.New("automation: unknown step id in next_ids")

// ErrEmptyAutomation is returned when an automation has no steps.
var ErrEmptyAutomation = errors.New("automation: no steps")

// ValidateDAG checks the structural invariants of an automation
// before it is persisted:
//
//   - at least one step
//   - step ids are unique
//   - every NextIDs reference points to an existing step
//   - the step graph is acyclic
//
// The traversal uses a depth-first search with a visiting map; the
// typical automation has fewer than 50 steps so the O(V+E) cost is
// negligible.
func ValidateDAG(a *types.Automation) error {
	if len(a.Steps) == 0 {
		return ErrEmptyAutomation
	}
	byID := make(map[string]*types.AutomationStep, len(a.Steps))
	for i := range a.Steps {
		s := &a.Steps[i]
		if s.ID == "" {
			return fmt.Errorf("%w: step %d missing id", ErrUnknownStep, i)
		}
		if _, dup := byID[s.ID]; dup {
			return fmt.Errorf("automation: duplicate step id %q", s.ID)
		}
		byID[s.ID] = s
	}
	for _, s := range a.Steps {
		for _, next := range s.NextIDs {
			if _, ok := byID[next]; !ok {
				return fmt.Errorf("%w: %s -> %s", ErrUnknownStep, s.ID, next)
			}
		}
	}

	// Detect cycles by DFS. We start from every step that has no
	// inbound edges (i.e., is a root) and walk the NextIDs; any
	// step still on stack when we revisit it indicates a cycle.
	const (
		unseen = iota
		onStack
		done
	)
	state := make(map[string]int, len(byID))
	var stack []string
	var dfs func(s string) error
	dfs = func(s string) error {
		state[s] = onStack
		stack = append(stack, s)
		for _, next := range byID[s].NextIDs {
			switch state[next] {
			case unseen:
				if err := dfs(next); err != nil {
					return err
				}
			case onStack:
				return fmt.Errorf("%w: %s -> %s", ErrCyclicDependency, s, next)
			}
		}
		stack = stack[:len(stack)-1]
		state[s] = done
		return nil
	}

	// Visit all roots (steps with no inbound edges). Steps with
	// inbound edges but in a separate disconnected component are
	// still walked via the second loop.
	for _, s := range a.Steps {
		if state[s.ID] == unseen {
			if err := dfs(s.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// TopologicalSort returns the step ids in execution order. It
// assumes ValidateDAG has already passed (no cycles, no unknown
// refs). Steps with no inbound edges come first; siblings preserve
// the order declared in the automation.
func TopologicalSort(a *types.Automation) []string {
	inbound := make(map[string]int, len(a.Steps))
	for _, s := range a.Steps {
		for _, next := range s.NextIDs {
			inbound[next]++
		}
	}
	var ready []string
	for _, s := range a.Steps {
		if inbound[s.ID] == 0 {
			ready = append(ready, s.ID)
		}
	}
	out := make([]string, 0, len(a.Steps))
	byID := make(map[string]*types.AutomationStep, len(a.Steps))
	for i := range a.Steps {
		byID[a.Steps[i].ID] = &a.Steps[i]
	}
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		out = append(out, id)
		for _, next := range byID[id].NextIDs {
			inbound[next]--
			if inbound[next] == 0 {
				ready = append(ready, next)
			}
		}
	}
	return out
}
