package weknora

import (
	"context"
	"net/url"

)

// ConversationService exposes /conversations list/get/delete.
type ConversationService struct{ c *Client }

// NewConversationService constructs a ConversationService.
func NewConversationService(c *Client) *ConversationService {
	return &ConversationService{c: c}
}

// List returns a single page of conversations.
func (s *ConversationService) List(ctx context.Context, pageSize int, pageToken string) (* ConversationPage, error) {
	q := url.Values{}
	if pageSize > 0 {
		q.Set("page_size", itoa(pageSize))
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	var out  ConversationPage
	if err := s.c.Do(ctx, "GET", "/conversations", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Iterate lazily fetches every conversation.
func (s *ConversationService) Iterate(ctx context.Context, pageSize int) * Iterator[ Conversation] {
	return  NewIterator(func(ctx context.Context, token string) ( Page[ Conversation], error) {
		page, err := s.List(ctx, pageSize, token)
		if err != nil {
			return  Page[ Conversation]{}, err
		}
		return  Page[ Conversation]{Items: page.Items, NextPageToken: page.NextPageToken}, nil
	})
}

// Get retrieves a conversation with its messages.
func (s *ConversationService) Get(ctx context.Context, conversationID string) (* Conversation, error) {
	var out  Conversation
	if err := s.c.Do(ctx, "GET", "/conversations/"+conversationID, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a conversation.
func (s *ConversationService) Delete(ctx context.Context, conversationID string) error {
	return s.c.Do(ctx, "DELETE", "/conversations/"+conversationID, nil, nil, nil)
}
