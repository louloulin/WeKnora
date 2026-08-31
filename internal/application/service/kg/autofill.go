package kg

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// AutofillService implements Tana-style AI Autofill: when a node adopts
// a Supertag, the AI suggests values for the schema's fields based on
// the surrounding text. This service is invoked asynchronously by the
// /supertags/:id/bind endpoint or by the Build #33 Automation on_add
// command (see types.KGSupertagCommand).
type AutofillService struct {
	llm LLMClient
}

// NewAutofillService constructs the Autofill service.
func NewAutofillService(llm LLMClient) *AutofillService {
	return &AutofillService{llm: llm}
}

// Suggest returns a property map populated by the LLM for the supplied
// Supertag fields. The returned map only contains keys the model is
// confident about (>= 0.5 confidence inferred from omission). When the
// LLM is unavailable, an empty map is returned (the caller treats that
// as "no suggestions").
func (a *AutofillService) Suggest(ctx context.Context, text string, supertag *types.KGSupertag, entityName string) (map[string]any, error) {
	if a.llm == nil || supertag == nil {
		return nil, nil
	}
	var fields []types.KGSupertagField
	_ = json.Unmarshal(supertag.Schema, &fields)
	if len(fields) == 0 {
		return nil, nil
	}
	fieldNames := make([]string, 0, len(fields))
	for _, f := range fields {
		fieldNames = append(fieldNames, f.Name)
	}
	system := "You fill missing fields for a typed entity. Output strict JSON only."
	user := fmt.Sprintf(`Entity name: %q
Supertag: %s
Fields: %s
Passage: %s

Return a JSON object with the field names as keys. Omit fields you cannot determine.`,
		entityName, supertag.Name, strings.Join(fieldNames, ", "), text)
	out, err := a.llm.Complete(ctx, system, user)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	out = strings.TrimPrefix(out, "```json")
	out = strings.TrimPrefix(out, "```")
	out = strings.TrimSuffix(out, "```")
	var props map[string]any
	if err := json.Unmarshal([]byte(out), &props); err != nil {
		return nil, fmt.Errorf("autofill: decode llm output: %w", err)
	}
	return props, nil
}
