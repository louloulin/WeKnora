// Package formula is the v0.7.26 Build #31 service layer that wires
// the formula engine to the multi-view database. It provides CRUD
// for formula columns and recomputation helpers.
package formula

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/formula"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// ErrCyclicDependency bubbles up from formula.DetectCycle.
var ErrCyclicDependency = formula.ErrCyclicDependency

// ErrInvalidExpression is returned when the formula does not parse.
var ErrInvalidExpression = errors.New("formula: invalid expression")

// Updater is the small subset of the database service that the
// formula service depends on. Decoupling lets the formula service be
// tested with a fake and avoids an import cycle on the database
// package.
type Updater interface {
	UpdateField(ctx context.Context, f *types.DatabaseField) error
}

// Service orchestrates formula columns.
type Service struct {
	db Updater
}

// NewService wires the service.
func NewService(db Updater) *Service { return &Service{db: db} }

// SetFormula installs a formula expression on a field. The field
// must already have type DatabaseFieldFormula. The options blob
// stores the expression, dependency list, and a monotonically
// increasing version so consumers can detect changes.
func (s *Service) SetFormula(ctx context.Context, f *types.DatabaseField, expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fmt.Errorf("%w: empty expression", ErrInvalidExpression)
	}
	// Validate the expression by parsing it once.
	toks, err := formula.NewLexer(expr).Tokenize()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidExpression, err)
	}
	if _, err := formula.NewParser(toks).Parse(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidExpression, err)
	}
	deps := formula.ExtractFieldRefs(expr)
	options := map[string]any{
		"expression": expr,
		"deps":       deps,
		"version":    1,
	}
	raw, _ := json.Marshal(options)
	f.Options = types.JSON(raw)
	if err := s.db.UpdateField(ctx, f); err != nil {
		return err
	}
	logger.Infof(ctx, "formula: set field_id=%s expr=%q deps=%v", f.ID, expr, deps)
	return nil
}

// EvaluateFormula computes the result of a formula expression given
// the row's current field values. Errors are returned to the caller
// for inspection but never written back to the row.
func (s *Service) EvaluateFormula(ctx context.Context, expr string, fields map[string]formula.Value) (formula.Value, error) {
	return formula.Evaluate(expr, formula.NewContext(fields))
}
