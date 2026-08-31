package weknora

import (
	"context"

)

// SearchService exposes hybrid BM25 + vector search.
type SearchService struct{ c *Client }

// NewSearchService constructs a SearchService bound to the given client.
func NewSearchService(c *Client) *SearchService {
	return &SearchService{c: c}
}

// Search runs a hybrid search and returns the hit list.
func (s *SearchService) Search(ctx context.Context, kbID string, req  SearchRequest) (* SearchResponse, error) {
	var out  SearchResponse
	if err := s.c.Do(ctx, "POST", "/knowledge-bases/"+kbID+"/search", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
