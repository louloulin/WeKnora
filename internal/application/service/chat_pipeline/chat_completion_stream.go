package chatpipeline

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// PluginChatCompletionStream implements streaming chat completion functionality
// as a plugin that can be registered to EventManager
type PluginChatCompletionStream struct {
	modelService interfaces.ModelService // Interface for model operations
}

// NewPluginChatCompletionStream creates a new PluginChatCompletionStream instance
// and registers it with the EventManager
func NewPluginChatCompletionStream(eventManager *EventManager,
	modelService interfaces.ModelService,
) *PluginChatCompletionStream {
	res := &PluginChatCompletionStream{
		modelService: modelService,
	}
	eventManager.Register(res)
	return res
}

// ActivationEvents returns the event types this plugin handles
func (p *PluginChatCompletionStream) ActivationEvents() []types.EventType {
	return []types.EventType{types.CHAT_COMPLETION_STREAM}
}

// OnEvent handles streaming chat completion events
// It prepares the chat model, messages, and initiates streaming response
func (p *PluginChatCompletionStream) OnEvent(ctx context.Context,
	eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError,
) *PluginError {
	pipelineInfo(ctx, "Stream", "input", map[string]interface{}{
		"session_id":     chatManage.SessionID,
		"user_question":  chatManage.UserContent,
		"history_rounds": len(chatManage.History),
		"chat_model":     chatManage.ChatModelID,
	})

	// Prepare chat model and options
	chatModel, opt, err := prepareChatModel(ctx, p.modelService, chatManage)
	if err != nil {
		return ErrGetChatModel.WithError(err)
	}

	// Prepare base messages without history

	chatMessages, modelContext := prepareMessagesWithModelContext(ctx, chatManage)
	chatMessages = modelContext.EncodeMessages(chatMessages)
	ctx = withPromptCacheMetadata(ctx, chatModel, chatMessages, opt, "knowledge_qa")
	pipelineInfo(ctx, "Stream", "messages_ready", map[string]interface{}{
		"message_count": len(chatMessages),
		"system_prompt": chatMessages[0].Content,
	})
	pipelineInfo(ctx, "Stream", "user_message", map[string]interface{}{
		"content": chatMessages[len(chatMessages)-1].Content,
	})
	// EventBus is required for event-driven streaming
	if chatManage.EventBus == nil {
		pipelineError(ctx, "Stream", "eventbus_missing", map[string]interface{}{
			"session_id": chatManage.SessionID,
		})
		return ErrModelCall.WithError(errors.New("EventBus is required for streaming"))
	}
	eventBus := chatManage.EventBus

	pipelineInfo(ctx, "Stream", "eventbus_ready", map[string]interface{}{
		"session_id": chatManage.SessionID,
	})

	// Initiate streaming chat model call with independent context
	pipelineInfo(ctx, "Stream", "model_call", map[string]interface{}{
		"chat_model": chatManage.ChatModelID,
	})
	responseChan, err := chatModel.ChatStream(ctx, chatMessages, opt)
	if err != nil {
		pipelineError(ctx, "Stream", "model_call", map[string]interface{}{
			"chat_model": chatManage.ChatModelID,
			"error":      err.Error(),
		})
		return ErrModelCall.WithError(err)
	}
	if responseChan == nil {
		pipelineError(ctx, "Stream", "model_call", map[string]interface{}{
			"chat_model": chatManage.ChatModelID,
			"error":      "nil_channel",
		})
		return ErrModelCall.WithError(errors.New("chat stream returned nil channel"))
	}

	pipelineInfo(ctx, "Stream", "model_started", map[string]interface{}{
		"session_id": chatManage.SessionID,
	})

	// Start goroutine to consume channel and emit events directly.
	// reasoning_content is routed to EventAgentThought (SSE response_type=thinking)
	// and plain answer text to EventAgentFinalAnswer, matching the Agent pipeline.
	// The goroutine monitors ctx.Done() to avoid leaking when the context is cancelled
	// and the upstream channel is not closed promptly.
	go func() {
		answerDecoder := modelContext.StreamDecoder()
		thinkingDecoder := modelContext.StreamDecoder()
		thinkingID := fmt.Sprintf("%s-thinking", uuid.New().String()[:8])
		answerID := fmt.Sprintf("%s-answer", uuid.New().String()[:8])
		thinkingOpen := false
		answerCompleted := false
		// citationBuilder (Build #30 B3) holds the de-dup map across chunks so
		// the same chunk_id cited in chunk 1 keeps position 1 when seen again
		// later. Gated by CitationsEnabled(); when off, Rewrite is identity
		// and CitationIndex stays nil on every emitted event.
		citations := newCitationBuilder(chatManage)

		closeThinking := func() {
			if !thinkingOpen {
				return
			}
			eventBus.Emit(ctx, types.Event{
				ID:        thinkingID,
				Type:      types.EventType(event.EventAgentThought),
				SessionID: chatManage.SessionID,
				Data: event.AgentThoughtData{
					Done: true,
				},
			})
			thinkingOpen = false
		}

		// emitFinalAnswer writes one terminal/delta AgentFinalAnswer event.
		// On the Done=true emit, the running CitationIndex is attached so
		// the IM layer can pair each [[cite:N]] token with the chunk the
		// audit handler will later log against (Build #30 B4).
		emitFinalAnswer := func(content string, done bool) {
			data := event.AgentFinalAnswerData{
				Content: content,
				Done:    done,
			}
			if done {
				if index := citations.Index(); index != nil {
					data.CitationIndex = index
				}
				// Persist on PipelineState for downstream plugins (audit
				// handler reads chatManage.CitationIndex).
				chatManage.CitationIndex = citations.Index()
			}
			_ = eventBus.Emit(ctx, types.Event{
				ID:        answerID,
				Type:      types.EventType(event.EventAgentFinalAnswer),
				SessionID: chatManage.SessionID,
				Data:      data,
			})
		}

		// flushDecoders drains any handle suffix the stream decoders held back to
		// bridge references split across provider chunks. Both the normal close
		// and the cancellation path must call this, otherwise a resource
		// reference in flight at teardown is silently dropped (and never
		// persisted, since the assistant message is saved from these events).
		flushDecoders := func() {
			thinkingTail := thinkingDecoder.Flush()
			if thinkingTail != "" {
				_ = eventBus.Emit(ctx, types.Event{
					ID:        thinkingID,
					Type:      types.EventType(event.EventAgentThought),
					SessionID: chatManage.SessionID,
					Data:      event.AgentThoughtData{Content: thinkingTail},
				})
			}
			answerTail := answerDecoder.Flush()
			if answerTail != "" {
				tail := citations.Rewrite(answerTail)
				// flushDecoders is teardown, not the terminal Done event.
				// Suppress the CitationIndex here so it only ships once, on
				// the explicit Done=true emit below.
				emitFinalAnswer(tail, false)
			}
		}

		for {
			select {
			case <-ctx.Done():
				flushDecoders()
				closeThinking()
				pipelineInfo(ctx, "Stream", "context_cancelled", map[string]interface{}{
					"session_id": chatManage.SessionID,
				})
				return

			case response, ok := <-responseChan:
				if !ok {
					flushDecoders()
					closeThinking()
					pipelineInfo(ctx, "Stream", "channel_close", map[string]interface{}{
						"session_id": chatManage.SessionID,
					})
					return
				}

				if response.ResponseType == types.ResponseTypeError {
					pipelineError(ctx, "Stream", "stream_error", map[string]interface{}{
						"session_id": chatManage.SessionID,
						"error":      response.Content,
					})
					eventBus.Emit(ctx, types.Event{
						ID:        fmt.Sprintf("%s-error", uuid.New().String()[:8]),
						Type:      types.EventType(event.EventError),
						SessionID: chatManage.SessionID,
						Data: event.ErrorData{
							Error:     response.Content,
							Stage:     "chat_completion_stream",
							SessionID: chatManage.SessionID,
						},
					})
					continue
				}

				if response.ResponseType == types.ResponseTypeThinking {
					response.Content = thinkingDecoder.Feed(response.Content)
					if response.Done {
						response.Content += thinkingDecoder.Flush()
					}
					if response.Content != "" {
						thinkingOpen = true
						eventBus.Emit(ctx, types.Event{
							ID:        thinkingID,
							Type:      types.EventType(event.EventAgentThought),
							SessionID: chatManage.SessionID,
							Data: event.AgentThoughtData{
								Content: response.Content,
								Done:    false,
							},
						})
					}
					if response.Done {
						closeThinking()
					}
					continue
				}

				if response.ResponseType == types.ResponseTypeAnswer {
					// Providers can emit a completion once for finish_reason and again
					// for their EOF sentinel. A final answer is a terminal event for a
					// single stream, so forwarding a later duplicate would put an answer
					// after the session's complete event.
					if answerCompleted {
						continue
					}
					response.Content = answerDecoder.Feed(response.Content)
					if response.Done {
						response.Content += answerDecoder.Flush()
						answerCompleted = true
					}
					closeThinking()
					emitFinalAnswer(citations.Rewrite(response.Content), response.Done)
				}
			}
		}
	}()

	return next()
}
