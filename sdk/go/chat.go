package weknora

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

)

// ChatService exposes RAG chat with one-shot (Ask) and streaming (Stream).
type ChatService struct{ c *Client }

// NewChatService constructs a ChatService.
func NewChatService(c *Client) *ChatService { return &ChatService{c: c} }

// Ask performs a one-shot RAG Q&A against the KB.
func (s *ChatService) Ask(ctx context.Context, kbID string, req  AskRequest) (* AskResponse, error) {
	var out  AskResponse
	if err := s.c.Do(ctx, "POST", "/knowledge-bases/"+kbID+"/ask", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Stream opens an SSE/NDJSON stream against /knowledge-bases/:id/chat and
// yields each ChatChunk through the returned channel. The channel is closed
// when the server signals 'done' or returns an error.
func (s *ChatService) Stream(ctx context.Context, kbID string, req  ChatRequest) (<-chan  ChatChunk, <-chan error) {
	chunks := make(chan  ChatChunk, 16)
	errs := make(chan error, 1)
	go func() {
		defer close(chunks)
		defer close(errs)
		req2, err := s.c.NewStreamRequest(ctx, "POST", "/knowledge-bases/"+kbID+"/chat", req)
		if err != nil {
			errs <- err
			return
		}
		resp, err := s.c.HTTP().Do(req2)
		if err != nil {
			errs <- err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			errs <- fmt.Errorf("weknora: chat stream: status %d", resp.StatusCode)
			return
		}
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var chunk  ChatChunk
			if err := json.Unmarshal(line, &chunk); err != nil {
				errs <- fmt.Errorf("weknora: chat stream decode: %w", err)
				return
			}
			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			case chunks <- chunk:
			}
			if chunk.Type ==  ChatChunkDone || chunk.Type ==  ChatChunkError {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errs <- fmt.Errorf("weknora: chat stream read: %w", err)
		}
	}()
	return chunks, errs
}

// Compile-time guard: keep http import even if downstream strips it.
var _ = http.StatusOK
