package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/models/vlm"
	"github.com/Tencent/WeKnora/internal/models/asr"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// fakeChatModel captures the last prompt + returns canned content so
// the service tests can verify action routing without standing up a
// real LLM.
type fakeChatModel struct {
	lastMessages []chat.Message
	lastOpts     *chat.ChatOptions
	content      string
	err          error
}

func (f *fakeChatModel) Chat(ctx context.Context, messages []chat.Message, opts *chat.ChatOptions) (*types.ChatResponse, error) {
	f.lastMessages = messages
	f.lastOpts = opts
	if f.err != nil {
		return nil, f.err
	}
	return &types.ChatResponse{Content: f.content}, nil
}

func (f *fakeChatModel) ChatStream(ctx context.Context, messages []chat.Message, opts *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	ch := make(chan types.StreamResponse, 1)
	ch <- types.StreamResponse{Content: f.content}
	close(ch)
	return ch, nil
}

func (f *fakeChatModel) GetModelID() string { return "fake-inline-ai" }

func (f *fakeChatModel) GetModelName() string { return "fake-inline-ai-name" }

// fakeModelService implements only the bits the inline AI service
// touches. Other methods panic — they should not be called from the
// service path under test.
type fakeModelService struct {
	defaults   []*types.Model
	all        []*types.Model
	resolveErr error
}

func (f *fakeModelService) ListModels(ctx context.Context) ([]*types.Model, error) {
	return f.all, nil
}

func (f *fakeModelService) GetChatModel(ctx context.Context, modelID string) (chat.Chat, error) {
	return &fakeChatModel{content: "ok"}, nil
}

func (f *fakeModelService) ClearModelCredential(_ context.Context, _, _ string) error { return nil }

// No-op stubs satisfy the rest of interfaces.ModelService. These are
// not exercised by the inline AI tests but the interface has grown
// over time and we need to satisfy the full contract.
func (f *fakeModelService) CreateModel(_ context.Context, _ *types.Model) error { return nil }
func (f *fakeModelService) GetModelByID(_ context.Context, id string) (*types.Model, error) {
	for _, m := range f.all {
		if m.ID == id {
			return m, nil
		}
	}
	return nil, nil
}
func (f *fakeModelService) UpdateModel(_ context.Context, _ *types.Model) error { return nil }
func (f *fakeModelService) DeleteModel(_ context.Context, _ string) error { return nil }
func (f *fakeModelService) UpdateModelCredentials(_ context.Context, _ string, _, _ *string) (*types.Model, error) {
	return nil, nil
}
func (f *fakeModelService) GetEmbeddingModel(_ context.Context, _ string) (embedding.Embedder, error) { return nil, nil }
func (f *fakeModelService) GetEmbeddingModelForTenant(_ context.Context, _ string, _ uint64) (embedding.Embedder, error) { return nil, nil }
func (f *fakeModelService) GetRerankModel(_ context.Context, _ string) (rerank.Reranker, error) { return nil, nil }
func (f *fakeModelService) GetVLMModel(_ context.Context, _ string) (vlm.VLM, error) { return nil, nil }
func (f *fakeModelService) GetASRModel(_ context.Context, _ string) (asr.ASR, error) { return nil, nil }
func (f *fakeModelService) Create(_ context.Context, _ *types.Model) error { return nil }
func (f *fakeModelService) GetByID(_ context.Context, _ uint64, id string) (*types.Model, error) {
	for _, m := range f.all {
		if m.ID == id {
			return m, nil
		}
	}
	return nil, nil
}
func (f *fakeModelService) Update(_ context.Context, _ *types.Model) error { return nil }
func (f *fakeModelService) Delete(_ context.Context, _ uint64, _ string) error { return nil }
func (f *fakeModelService) ClearDefaultByType(_ context.Context, _ uint64, _ types.ModelType, _ string) error { return nil }
func (f *fakeModelService) List(_ context.Context, _, _, _ interface{}) ([]*types.Model, error) { return f.all, nil }


func TestInlineAI_Run_HappyPath(t *testing.T) {
	fm := &fakeChatModel{content: "Short summary"}
	svc := newInlineAIServiceWith(&fakeModelService{all: []*types.Model{
		{ID: "m1", Type: types.ModelTypeKnowledgeQA, IsDefault: true},
	}}, fm)

	resp, err := svc.Run(context.Background(), 1, types.InlineAIRequest{
		Action: types.InlineAISummarize,
		Text:   "Long paragraph about WeKnora that should be summarized.",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Result != "Short summary" {
		t.Fatalf("result = %q, want %q", resp.Result, "Short summary")
	}
	if resp.Action != types.InlineAISummarize {
		t.Fatalf("action = %v, want %v", resp.Action, types.InlineAISummarize)
	}
	if resp.Model != "m1" {
		t.Fatalf("model = %q, want m1", resp.Model)
	}
	if len(fm.lastMessages) != 2 {
		t.Fatalf("messages = %d, want 2 (system+user)", len(fm.lastMessages))
	}
	if !strings.Contains(fm.lastMessages[0].Content, "summarizer") {
		t.Fatalf("system prompt missing summarizer tone: %s", fm.lastMessages[0].Content)
	}
}

func TestInlineAI_Run_RejectsBadInput(t *testing.T) {
	svc := newInlineAIServiceWith(&fakeModelService{all: []*types.Model{
		{ID: "m1", Type: types.ModelTypeKnowledgeQA, IsDefault: true},
	}}, &fakeChatModel{})

	cases := []struct {
		name string
		req  types.InlineAIRequest
	}{
		{"empty text", types.InlineAIRequest{Action: types.InlineAISummarize}},
		{"unknown action", types.InlineAIRequest{Action: "nonsense", Text: "x"}},
		{"too long", types.InlineAIRequest{Action: types.InlineAISummarize, Text: strings.Repeat("x", 17*1024)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.Run(context.Background(), 1, c.req)
			if !errors.Is(err, types.ErrInlineAIBadInput) {
				t.Fatalf("expected ErrInlineAIBadInput, got %v", err)
			}
		})
	}
}

func TestInlineAI_Run_FallsBackToAnyChatModel(t *testing.T) {
	svc := newInlineAIServiceWith(&fakeModelService{all: []*types.Model{
		{ID: "embedding-only", Type: types.ModelTypeEmbedding},
		{ID: "m2", Type: types.ModelTypeKnowledgeQA},
	}}, &fakeChatModel{content: "ok"})

	resp, err := svc.Run(context.Background(), 1, types.InlineAIRequest{
		Action: types.InlineAIExplain,
		Text:   "Explain this.",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Model != "m2" {
		t.Fatalf("fallback model = %q, want m2", resp.Model)
	}
}

func TestInlineAI_Run_NoModelReturnsUnavailable(t *testing.T) {
	svc := newInlineAIServiceWith(&fakeModelService{}, &fakeChatModel{})

	_, err := svc.Run(context.Background(), 1, types.InlineAIRequest{
		Action: types.InlineAIRewrite,
		Text:   "Rewrite me.",
	})
	if !errors.Is(err, interfaces.ErrInlineAIUnavailable) {
		t.Fatalf("expected ErrInlineAIUnavailable, got %v", err)
	}
}

func TestInlineAI_TranslateDefaultsToEnglish(t *testing.T) {
	fm := &fakeChatModel{content: "Bonjour"}
	svc := newInlineAIServiceWith(&fakeModelService{all: []*types.Model{
		{ID: "m1", Type: types.ModelTypeKnowledgeQA, IsDefault: true},
	}}, fm)

	_, err := svc.Run(context.Background(), 1, types.InlineAIRequest{
		Action: types.InlineAITranslate,
		Text:   "Hello",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(fm.lastMessages[0].Content, "English") {
		t.Fatalf("translate prompt missing default language: %s", fm.lastMessages[0].Content)
	}
}

func TestInlineAI_InstructionPipesIntoUserPrompt(t *testing.T) {
	fm := &fakeChatModel{content: "x"}
	svc := newInlineAIServiceWith(&fakeModelService{all: []*types.Model{
		{ID: "m1", Type: types.ModelTypeKnowledgeQA, IsDefault: true},
	}}, fm)

	_, err := svc.Run(context.Background(), 1, types.InlineAIRequest{
		Action:      types.InlineAIRewrite,
		Text:        "Some text",
		Instruction: "in a friendly tone",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(fm.lastMessages[1].Content, "friendly tone") {
		t.Fatalf("user prompt missing instruction: %s", fm.lastMessages[1].Content)
	}
}

// newInlineAIServiceWith is a small helper that bypasses the public
// constructor so the test can pass a fake model service + fake chat
// model separately.
func newInlineAIServiceWith(ms interfaces.ModelService, fm chat.Chat) *inlineAIService {
	return &inlineAIService{modelService: ms}
}
