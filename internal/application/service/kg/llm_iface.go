// Package kg implements the Build #35 Knowledge Graph + Supertag engine.
package kg

import "context"

// LLMClient is the minimal LLM contract used by the NER and RE pipelines.
// The interface is deliberately tiny so we can swap in any backend (the
// existing ChatModel abstraction, OpenAI, DeepSeek, Doubao, ...) without
// changing the pipeline code.
type LLMClient interface {
	// Complete sends a system + user prompt and returns the raw model
	// output (a JSON string the caller is expected to parse).
	Complete(ctx context.Context, system, user string) (string, error)
}
