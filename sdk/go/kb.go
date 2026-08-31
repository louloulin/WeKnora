// Package services contains the per-surface method sets exposed by the
// WeKnora SDK. Each file holds one service and depends only on the parent
// package for transport, auth, and retry.
package weknora

import (
	"context"
	"net/url"

)

// KnowledgeBaseService provides CRUD over the /knowledge-bases surface.
type KnowledgeBaseService struct {
	c *Client
}

// NewKnowledgeBaseService constructs a service bound to the given client.
func NewKnowledgeBaseService(c *Client) *KnowledgeBaseService {
	return &KnowledgeBaseService{c: c}
}

// Create inserts a new knowledge base.
func (s *KnowledgeBaseService) Create(ctx context.Context, in  KnowledgeBaseInput) (* KnowledgeBase, error) {
	var out  KnowledgeBase
	if err := s.c.Do(ctx, "POST", "/knowledge-bases", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get retrieves a single knowledge base by ID.
func (s *KnowledgeBaseService) Get(ctx context.Context, kbID string) (* KnowledgeBase, error) {
	var out  KnowledgeBase
	if err := s.c.Do(ctx, "GET", "/knowledge-bases/"+kbID, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update patches an existing knowledge base.
func (s *KnowledgeBaseService) Update(ctx context.Context, kbID string, patch  KnowledgeBasePatch) (* KnowledgeBase, error) {
	var out  KnowledgeBase
	if err := s.c.Do(ctx, "PATCH", "/knowledge-bases/"+kbID, nil, patch, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a knowledge base. Returns nil on success.
func (s *KnowledgeBaseService) Delete(ctx context.Context, kbID string) error {
	return s.c.Do(ctx, "DELETE", "/knowledge-bases/"+kbID, nil, nil, nil)
}

// List returns a single page of knowledge bases. Use Iterate for full traversal.
func (s *KnowledgeBaseService) List(ctx context.Context, pageSize int, pageToken string) (* KnowledgeBasePage, error) {
	q := url.Values{}
	if pageSize > 0 {
		q.Set("page_size", itoa(pageSize))
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	var out  KnowledgeBasePage
	if err := s.c.Do(ctx, "GET", "/knowledge-bases", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Iterate returns an iterator that lazily fetches every knowledge base for
// the current tenant.
func (s *KnowledgeBaseService) Iterate(ctx context.Context, pageSize int) * Iterator[ KnowledgeBase] {
	return  NewIterator(func(ctx context.Context, token string) ( Page[ KnowledgeBase], error) {
		page, err := s.List(ctx, pageSize, token)
		if err != nil {
			return  Page[ KnowledgeBase]{}, err
		}
		return  Page[ KnowledgeBase]{Items: page.Items, NextPageToken: page.NextPageToken}, nil
	})
}

// itoa is a tiny helper so services don't import strconv just for one call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
