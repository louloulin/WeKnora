package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// InlineAIService is the business-logic facade for paragraph-level AI
// actions (summarize / translate / rewrite / explain / extract_task /
// generate_table). It is intentionally narrow — the handler can swap
// any implementation without rewiring the route.
type InlineAIService interface {
	// Run dispatches the request to the appropriate system prompt +
	// chat model. Returns the model's reply wrapped in a typed
	// InlineAIResponse. Errors:
	//   - types.ErrInlineAIBadInput (wrapped to 400 at handler layer)
	//   - ErrInlineAIUnavailable   (no chat model available)
	//   - any underlying LLM error (wrapped to 502)
	Run(ctx context.Context, tenantID uint64, req types.InlineAIRequest) (*types.InlineAIResponse, error)
}
