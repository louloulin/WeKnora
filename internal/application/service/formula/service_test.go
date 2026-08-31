package formula

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/formula"
	"github.com/Tencent/WeKnora/internal/types"
)

type fakeUpdater struct {
	lastField *types.DatabaseField
	failNext  bool
}

func (f *fakeUpdater) UpdateField(ctx context.Context, field *types.DatabaseField) error {
	if f.failNext {
		return errors.New("update failed")
	}
	f.lastField = field
	return nil
}

func TestSetFormula_StoresExpressionAndDeps(t *testing.T) {
	updater := &fakeUpdater{}
	svc := NewService(updater)
	field := &types.DatabaseField{ID: "f1", Name: "Total", Type: types.DatabaseFieldFormula}
	err := svc.SetFormula(context.Background(), field, "$price * 1.1")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if updater.lastField == nil {
		t.Fatal("updater not called")
	}
	if len(updater.lastField.Options) == 0 {
		t.Fatal("options not populated")
	}
}

func TestSetFormula_RejectsEmpty(t *testing.T) {
	svc := NewService(&fakeUpdater{})
	err := svc.SetFormula(context.Background(), &types.DatabaseField{ID: "f1"}, "  ")
	if err == nil {
		t.Fatal("expected error on empty expression")
	}
}

func TestSetFormula_RejectsSyntaxError(t *testing.T) {
	svc := NewService(&fakeUpdater{})
	err := svc.SetFormula(context.Background(), &types.DatabaseField{ID: "f1"}, "$a +")
	if err == nil {
		t.Fatal("expected error on syntax error")
	}
}

func TestSetFormula_PropagatesUpdateError(t *testing.T) {
	updater := &fakeUpdater{failNext: true}
	svc := NewService(updater)
	err := svc.SetFormula(context.Background(), &types.DatabaseField{ID: "f1"}, "$a + $b")
	if err == nil {
		t.Fatal("expected error from updater")
	}
}

func TestEvaluateFormula_DelegatesToEngine(t *testing.T) {
	svc := NewService(&fakeUpdater{})
	v, err := svc.EvaluateFormula(context.Background(),
		"round($price * 1.1, 0)",
		map[string]formula.Value{"price": {Kind: formula.ValueNumber, Num: 100}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if v.Num != 110 {
		t.Errorf("got %v want 110", v.Num)
	}
}
