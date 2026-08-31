package formula

import (
	"sort"
	"testing"
)

func TestExtractFieldRefs(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"$price * 1.1", []string{"price"}},
		{"$a + $b - $c", []string{"a", "b", "c"}},
		{"sum($items) + avg($items)", []string{"items"}},
		{"$user.profile.name", []string{"user"}}, // dot chain still produces get($user, "name") so $user is a ref
		{"if($active, $name, $default)", []string{"active", "default", "name"}},
		{"1 + 2", nil}, // no field refs
		{"plain_ident + 1", []string{"plain_ident"}}, // bare identifier is also a ref
	}
	for _, c := range cases {
		got := ExtractFieldRefs(c.src)
		want := append([]string(nil), c.want...)
		sort.Strings(want)
		if len(got) != len(want) {
			t.Errorf("%s: got %v want %v", c.src, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: field %d got %s want %s", c.src, i, got[i], want[i])
			}
		}
	}
}

func TestDetectCycle(t *testing.T) {
	graph := map[string][]string{
		"a": {"b"},
		"b": {"c"},
	}
	if !DetectCycle(graph, "c", "a") {
		t.Error("expected cycle when c -> a")
	}
	if DetectCycle(graph, "d", "a") {
		t.Error("expected no cycle when d -> a (independent)")
	}
	// Self-loop.
	if !DetectCycle(graph, "a", "a") {
		t.Error("expected self-loop cycle a -> a")
	}
}
