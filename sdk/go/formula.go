package weknora

import (
	"context"

)

// FormulaService exposes the Build #32 formula engine for inline evaluation.
type FormulaService struct{ c *Client }

// NewFormulaService constructs a FormulaService.
func NewFormulaService(c *Client) *FormulaService { return &FormulaService{c: c} }

// Eval evaluates a formula expression in the supplied context.
func (s *FormulaService) Eval(ctx context.Context, kbID string, req  FormulaEvalRequest) (* FormulaEvalResponse, error) {
	var out  FormulaEvalResponse
	if err := s.c.Do(ctx, "POST", "/knowledge-bases/"+kbID+"/formula/eval", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
