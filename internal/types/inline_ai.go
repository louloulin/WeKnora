package types

import (
	"errors"
	"strings"
)

// InlineAIAction enumerates the supported paragraph-level AI actions.
// Each action has its own system prompt (see service/inline_ai.go).
// Keep the enum stable — values are persisted in audit logs.
type InlineAIAction string

const (
	InlineAISummarize  InlineAIAction = "summarize"
	InlineAITranslate  InlineAIAction = "translate"
	InlineAIRewrite    InlineAIAction = "rewrite"
	InlineAIExplain    InlineAIAction = "explain"
	InlineAIExtractTask InlineAIAction = "extract_task"
	InlineAIGenerateTable InlineAIAction = "generate_table"
)

// AllInlineAIActions is the canonical, ordered list exposed to the UI
// slash / selection menu. The order doubles as the default rendering
// order; the frontend may reorder but should never drop any entry.
var AllInlineAIActions = []InlineAIAction{
	InlineAISummarize,
	InlineAITranslate,
	InlineAIRewrite,
	InlineAIExplain,
	InlineAIExtractTask,
	InlineAIGenerateTable,
}

// IsValidInlineAIAction reports whether a is one of the supported
// actions. Used by the handler to reject malformed requests with 400.
func IsValidInlineAIAction(a InlineAIAction) bool {
	switch a {
	case InlineAISummarize, InlineAITranslate, InlineAIRewrite,
		InlineAIExplain, InlineAIExtractTask, InlineAIGenerateTable:
		return true
	}
	return false
}

// InlineAIMaxInputBytes caps the input text size at 16 KB. Larger
// selections are rejected at the request level so a runaway client
// cannot blow up the LLM quota with a multi-megabyte payload.
const InlineAIMaxInputBytes = 16 * 1024

// InlineAIMaxOutputTokens is the default completion cap applied when
// the caller does not pass max_tokens. 1024 is enough for any of the
// 6 actions; the LLM short-circuits well before that for short
// instructions.
const InlineAIMaxOutputTokens = 1024

// InlineAIRequest is the body accepted by POST /api/v1/ai/inline.
// ModelID is optional; when empty the service resolves the tenant's
// default knowledge-QA model. Instruction is an optional free-form
// prompt (e.g. "translate to English") that overrides the action's
// default behaviour. TargetLanguage is required for translate.
type InlineAIRequest struct {
	Action         InlineAIAction `json:"action" binding:"required"`
	Text           string         `json:"text" binding:"required"`
	ModelID        string         `json:"model_id,omitempty"`
	Instruction    string         `json:"instruction,omitempty"`
	TargetLanguage string         `json:"target_language,omitempty"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
}

// Normalize trims whitespace, lower-cases the action, and fills
// MaxTokens from the package default if zero. Returns ErrInlineAIBadInput
// when validation fails — the handler maps that to 400.
func (r *InlineAIRequest) Normalize() error {
	r.Action = InlineAIAction(strings.ToLower(strings.TrimSpace(string(r.Action))))
	if !IsValidInlineAIAction(r.Action) {
		return ErrInlineAIBadInput
	}
	r.Text = strings.TrimSpace(r.Text)
	if r.Text == "" {
		return ErrInlineAIBadInput
	}
	if len(r.Text) > InlineAIMaxInputBytes {
		return ErrInlineAIBadInput
	}
	r.Instruction = strings.TrimSpace(r.Instruction)
	r.TargetLanguage = strings.TrimSpace(r.TargetLanguage)
	if r.Action == InlineAITranslate && r.TargetLanguage == "" {
		r.TargetLanguage = "English"
	}
	if r.MaxTokens <= 0 {
		r.MaxTokens = InlineAIMaxOutputTokens
	}
	if r.MaxTokens > 4096 {
		r.MaxTokens = 4096
	}
	return nil
}

// InlineAIResponse is the JSON envelope returned by the inline AI
// endpoint. Result is the raw LLM output; Usage captures token counts
// when the underlying provider reports them; Action + Model echo back
// for the audit log.
type InlineAIResponse struct {
	Action      InlineAIAction `json:"action"`
	Model       string         `json:"model"`
	Result      string         `json:"result"`
	InputTokens int            `json:"input_tokens,omitempty"`
	OutputTokens int           `json:"output_tokens,omitempty"`
}

// ErrInlineAIBadInput is returned by Normalize when the request shape
// is invalid. Defined alongside the types so the service can translate
// it to a sentinel error that the handler maps to 400.
var ErrInlineAIBadInput = errors.New("inline ai: bad input")
