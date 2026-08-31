package automation

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestValidateDAG_Empty(t *testing.T) {
	a := &types.Automation{}
	if err := ValidateDAG(a); err != ErrEmptyAutomation {
		t.Errorf("err = %v, want ErrEmptyAutomation", err)
	}
}

func TestValidateDAG_DuplicateID(t *testing.T) {
	a := &types.Automation{
		Steps: []types.AutomationStep{
			{ID: "a", Name: "A", ActionType: types.AutomationActionUpdateField},
			{ID: "a", Name: "A2", ActionType: types.AutomationActionUpdateField},
		},
	}
	if err := ValidateDAG(a); err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestValidateDAG_UnknownNextRef(t *testing.T) {
	a := &types.Automation{
		Steps: []types.AutomationStep{
			{ID: "a", Name: "A", ActionType: types.AutomationActionUpdateField, NextIDs: []string{"missing"}},
		},
	}
	if err := ValidateDAG(a); err == nil {
		t.Fatal("expected unknown step ref error")
	}
}

func TestValidateDAG_Cycle(t *testing.T) {
	a := &types.Automation{
		Steps: []types.AutomationStep{
			{ID: "a", Name: "A", ActionType: types.AutomationActionUpdateField, NextIDs: []string{"b"}},
			{ID: "b", Name: "B", ActionType: types.AutomationActionUpdateField, NextIDs: []string{"a"}},
		},
	}
	err := ValidateDAG(a)
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if err.Error() == "" {
		t.Fatal("empty error message")
	}
}

func TestValidateDAG_NoCycleLinear(t *testing.T) {
	a := &types.Automation{
		Steps: []types.AutomationStep{
			{ID: "a", Name: "A", ActionType: types.AutomationActionUpdateField, NextIDs: []string{"b"}},
			{ID: "b", Name: "B", ActionType: types.AutomationActionUpdateField, NextIDs: []string{"c"}},
			{ID: "c", Name: "C", ActionType: types.AutomationActionUpdateField},
		},
	}
	if err := ValidateDAG(a); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateDAG_Diamond(t *testing.T) {
	// a -> b, a -> c, b -> d, c -> d (diamond shape, no cycle)
	a := &types.Automation{
		Steps: []types.AutomationStep{
			{ID: "a", ActionType: types.AutomationActionUpdateField, NextIDs: []string{"b", "c"}},
			{ID: "b", ActionType: types.AutomationActionUpdateField, NextIDs: []string{"d"}},
			{ID: "c", ActionType: types.AutomationActionUpdateField, NextIDs: []string{"d"}},
			{ID: "d", ActionType: types.AutomationActionUpdateField},
		},
	}
	if err := ValidateDAG(a); err != nil {
		t.Fatalf("diamond should not be a cycle: %v", err)
	}
}

func TestTopologicalSort_Linear(t *testing.T) {
	a := &types.Automation{
		Steps: []types.AutomationStep{
			{ID: "a", ActionType: types.AutomationActionUpdateField, NextIDs: []string{"b"}},
			{ID: "b", ActionType: types.AutomationActionUpdateField, NextIDs: []string{"c"}},
			{ID: "c", ActionType: types.AutomationActionUpdateField},
		},
	}
	got := TopologicalSort(a)
	want := []string{"a", "b", "c"}
	if !equal(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestTopologicalSort_Diamond(t *testing.T) {
	a := &types.Automation{
		Steps: []types.AutomationStep{
			{ID: "a", ActionType: types.AutomationActionUpdateField, NextIDs: []string{"b", "c"}},
			{ID: "b", ActionType: types.AutomationActionUpdateField, NextIDs: []string{"d"}},
			{ID: "c", ActionType: types.AutomationActionUpdateField, NextIDs: []string{"d"}},
			{ID: "d", ActionType: types.AutomationActionUpdateField},
		},
	}
	got := TopologicalSort(a)
	if len(got) != 4 {
		t.Errorf("got %v", got)
	}
	if got[0] != "a" {
		t.Errorf("first should be a, got %v", got)
	}
	if got[3] != "d" {
		t.Errorf("last should be d, got %v", got)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
