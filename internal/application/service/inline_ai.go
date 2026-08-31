package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// NewInlineAIService returns an InlineAIService backed by the supplied
// model service. The model service is required because every action
// ultimately resolves through modelService.GetChatModel.
func NewInlineAIService(modelService interfaces.ModelService) interfaces.InlineAIService {
	return &inlineAIService{modelService: modelService}
}

type inlineAIService struct {
	modelService interfaces.ModelService
}

// Run validates the request, picks the chat model (caller-provided or
// tenant default), and dispatches to the action-specific system prompt.
// The LLM call is non-streaming today; streaming can be added later by
// switching Chat → ChatStream without changing the wire shape.
func (s *inlineAIService) Run(
	ctx context.Context,
	tenantID uint64,
	req types.InlineAIRequest,
) (*types.InlineAIResponse, error) {
	if err := req.Normalize(); err != nil {
		return nil, err
	}

	modelID, err := s.resolveModelID(ctx, tenantID, req.ModelID)
	if err != nil {
		return nil, err
	}

	chatModel, err := s.modelService.GetChatModel(ctx, modelID)
	if err != nil || chatModel == nil {
		logger.Errorf(ctx, "inline ai: failed to get chat model: id=%s err=%v", modelID, err)
		return nil, interfaces.ErrInlineAIUnavailable
	}

	systemPrompt := systemPromptFor(req)
	userPrompt := userPromptFor(req)

	messages := []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	chatOpts := &chat.ChatOptions{
		MaxTokens: req.MaxTokens,
	}

	start := time.Now()
	resp, err := chatModel.Chat(ctx, messages, chatOpts)
	if err != nil {
		logger.Errorf(ctx, "inline ai: chat call failed: action=%s model=%s err=%v", req.Action, modelID, err)
		return nil, fmt.Errorf("inline ai: chat failed: %w", err)
	}

	result := strings.TrimSpace(resp.Content)
	if result == "" {
		return nil, errors.New("inline ai: empty response from model")
	}

	logger.Infof(ctx, "inline ai: action=%s model=%s in=%d out=%d latency=%s",
		req.Action, modelID, estimateTokens(req.Text), estimateTokens(result), time.Since(start))

	out := &types.InlineAIResponse{
		Action: req.Action,
		Model:  modelID,
		Result: result,
	}
	if resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
		out.InputTokens = resp.Usage.PromptTokens
		out.OutputTokens = resp.Usage.CompletionTokens
	}
	return out, nil
}

// resolveModelID returns req.ModelID when set; otherwise it queries
// the model repository for the tenant's default knowledge-QA model.
// Returns ErrInlineAIUnavailable when neither path yields a usable id.
func (s *inlineAIService) resolveModelID(ctx context.Context, tenantID uint64, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return requested, nil
	}
	if s.modelService == nil {
		return "", interfaces.ErrInlineAIUnavailable
	}
	// Try each default chat model until one resolves. The container
	// wires the model service with tenant filtering so the lookup
	// stays scoped.
	models, err := s.modelService.ListModels(ctx)
	if err == nil {
		for _, m := range models {
			if m == nil {
				continue
			}
			if m.Type == types.ModelTypeKnowledgeQA && m.IsDefault {
				return m.ID, nil
			}
		}
		// Fallback: any chat model the tenant owns.
		for _, m := range models {
			if m == nil {
				continue
			}
			if m.Type == types.ModelTypeKnowledgeQA {
				return m.ID, nil
			}
		}
	}
	logger.Errorf(ctx, "inline ai: no default chat model for tenant=%d err=%v", tenantID, err)
	return "", interfaces.ErrInlineAIUnavailable
}

// systemPromptFor returns the action-specific system prompt. Each
// prompt is intentionally tight so a cheap model still produces a
// usable reply. Override the action's default behaviour by passing
// req.Instruction — the user prompt section weaves it in.
func systemPromptFor(req types.InlineAIRequest) string {
	switch req.Action {
	case types.InlineAISummarize:
		return "You are a concise technical summarizer. Produce a 1-3 sentence summary that captures the key facts of the user's text. Do not add commentary. Reply in the same language as the input unless the user provides an instruction."
	case types.InlineAITranslate:
		lang := req.TargetLanguage
		if lang == "" {
			lang = "English"
		}
		return fmt.Sprintf("You are a professional translator. Translate the user's text into %s. Preserve the original meaning, tone, and any technical terms. Reply with only the translated text — no preamble.", lang)
	case types.InlineAIRewrite:
		return "You are a precise copy-editor. Rewrite the user's text to be clearer, more concise, and free of ambiguity. Keep the meaning intact. Do not add new content. Reply with only the rewritten text."
	case types.InlineAIExplain:
		return "You are a patient technical explainer. Explain the user's text in plain language. Start with a one-sentence TL;DR, then expand with concrete details. Use markdown lists when appropriate. Reply in the same language as the input."
	case types.InlineAIExtractTask:
		return "You extract actionable tasks from free-form text. Reply in markdown with one bullet per task, formatted as `- [ ] <verb> <object>`. Skip sentences that are not tasks. Keep tasks short and concrete. Reply in the same language as the input."
	case types.InlineAIGenerateTable:
		return "You convert prose into structured tables. Reply with a single markdown table whose columns capture the most useful facets of the content. Keep cell values short (≤ 12 words). Reply with only the table — no preamble."
	default:
		return "You are a helpful assistant. Reply concisely."
	}
}

// userPromptFor assembles the user-role prompt. When the caller passes
// a custom instruction we route it into the system prompt slot rather
// than the user slot, so the action contract still governs the model.
func userPromptFor(req types.InlineAIRequest) string {
	if req.Instruction != "" {
		// Replace the user content with: "<instruction>\n\n<text>"
		return fmt.Sprintf("%s\n\n---\n\n%s", req.Instruction, req.Text)
	}
	return req.Text
}

// estimateTokens returns a rough token count for log lines. Real token
// counting requires the model's tokenizer; this estimator assumes ~4
// characters per token, which is accurate enough for ASCII / CJK mixed
// prose.
func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 3) / 4
}

// Compile-time assertion.
var _ interfaces.InlineAIService = (*inlineAIService)(nil)
