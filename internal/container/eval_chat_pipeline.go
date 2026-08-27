package container

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// Build #31 — default EvalChatPipeline adapter.
//
// The runner accepts any implementation of service.EvalChatPipeline;
// this file ships the production adapter that delegates to the chat
// completion / search / reflection stack.
//
// Why a separate adapter file:
//   - Keeps container.go (which is already >1300 lines) clean.
//   - Lets us wire the production pipeline once and have tests
//     override it via DI without touching container.go.
//   - Future PRs can swap the adapter (e.g. wire to an isolated
//     agent-only chat model) without rewriting the runner.

// defaultEvalChatPipeline is the production EvalChatPipeline. It is a
// thin adapter that, in B31, returns a stubbed response. The full
// chat pipeline integration lands in a follow-up build that wires
// chat_manage + the reflection event log to the runner — keeping B31
// ship-able while the chat pipeline touches a separate PR.
type defaultEvalChatPipeline struct {
	db        *gorm.DB
	modelSvc  interfaces.ModelService
}

// newDefaultEvalChatPipeline wires the production adapter.
func newDefaultEvalChatPipeline(db *gorm.DB, modelSvc interfaces.ModelService) service.EvalChatPipeline {
	return &defaultEvalChatPipeline{db: db, modelSvc: modelSvc}
}

// AnswerQA returns the stub response. The full pipeline integration
// lands in B31.x — for B31 the runner exercises the judge / summary /
// badcase machinery against a deterministic stub so the harness can
// pin behaviour without depending on a live chat backend.
//
// The stub returns:
//   - model_answer: <echo of question>  → factuality=1
//   - search_top_k: []
//   - citation_index: {}
//   - reflection_events: []
//
// This shape lets the harness exercise every judge branch without
// requiring a chat model in CI.
func (p *defaultEvalChatPipeline) AnswerQA(ctx context.Context, req service.EvalChatRequest) (*service.EvalChatResponse, error) {
	if req.Question == "" {
		return nil, errors.New("question is required")
	}
	// Sanity-check the model id is non-empty — the chat pipeline is
	// resolved by the modelSvc when chat integration lands.
	if req.ChatModelID == "" {
		return nil, errors.New("chat_model_id is required")
	}
	stubAnswer := fmt.Sprintf("[eval stub] %s", req.Question)
	resp := &service.EvalChatResponse{
		ModelAnswer:       stubAnswer,
		SearchTopK:        json.RawMessage(`[]`),
		CitationIndex:     json.RawMessage(`{}`),
		ReflectionEvents:  json.RawMessage(`[]`),
	}
	return resp, nil
}

// compile-time guard that the adapter satisfies the seam.
var _ service.EvalChatPipeline = (*defaultEvalChatPipeline)(nil)