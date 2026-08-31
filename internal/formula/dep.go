package formula

import (
	"sort"
	"strings"
)

// ExtractFieldRefs returns the set of column names that the
// expression depends on, in lexical order with duplicates removed.
// A bare identifier (e.g. `price`) is also treated as a field
// reference so callers do not need to remember the "$" prefix.
//
// The extraction is performed against the parsed AST so it is
// consistent with what the evaluator will read at runtime.
func ExtractFieldRefs(src string) []string {
	toks, err := NewLexer(src).Tokenize()
	if err != nil {
		return nil
	}
	node, err := NewParser(toks).Parse()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	collectFieldRefs(node, seen)
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// CollectFieldRefs walks a pre-parsed AST and writes every
// referenced field name into seen. The lower-case key is stored so
// field lookups stay case-insensitive.
func CollectFieldRefs(n Node, seen map[string]bool) {
	collectFieldRefs(n, seen)
}

func collectFieldRefs(n Node, seen map[string]bool) {
	switch n.Kind {
	case NodeFieldRef:
		if len(n.Chain) > 0 {
			seen[strings.ToLower(n.Chain[0])] = true
		}
	case NodeIdent:
		// Bare identifier at expression position: treat as field ref.
		seen[strings.ToLower(n.Ident)] = true
	case NodeUnary:
		collectFieldRefs(*n.Operand, seen)
	case NodeBinary:
		collectFieldRefs(*n.LHS, seen)
		collectFieldRefs(*n.RHS, seen)
	case NodeCall:
		for i := range n.Args {
			collectFieldRefs(n.Args[i], seen)
		}
	case NodeList:
		for i := range n.Elems {
			collectFieldRefs(n.Elems[i], seen)
		}
	case NodeIf:
		collectFieldRefs(*n.Cond, seen)
		collectFieldRefs(*n.Then, seen)
		collectFieldRefs(*n.Else, seen)
	}
}

// DetectCycle reports whether adding an edge from→to to the existing
// graph would form a cycle. The graph maps a column name to the
// columns it depends on.
//
// This is a lightweight O(V+E) DFS that is sufficient for the
// typical handful of formula columns per database.
func DetectCycle(graph map[string][]string, from, to string) bool {
	// Add the candidate edge temporarily.
	graph[from] = append(graph[from], to)
	defer func() {
		// Remove the last element we appended.
		if deps := graph[from]; len(deps) > 0 {
			graph[from] = deps[:len(deps)-1]
			if len(graph[from]) == 0 {
				delete(graph, from)
			}
		}
	}()
	visited := map[string]bool{}
	var dfs func(node string) bool
	dfs = func(node string) bool {
		if visited[node] {
			return true
		}
		visited[node] = true
		for _, dep := range graph[node] {
			if dep == from || dfs(dep) {
				return true
			}
		}
		return false
	}
	return dfs(to)
}
