package weknora

import (
	"context"
	"errors"
)

// Page represents a single page of results returned by paginated list
// endpoints. Endpoints that don't expose next_page_token must still
// satisfy this shape (returning an empty token when done).
type Page[T any] struct {
	Items         []T
	NextPageToken string
}

// Iterator yields pages of T until NextPageToken is empty. It satisfies the
// same AsyncIterable pattern as the Notion / Tana SDKs so callers can use
// range loops over channels without leaking goroutines.
type Iterator[T any] struct {
	fetch   func(ctx context.Context, token string) (Page[T], error)
	token   string
	done    bool
	err     error
	pending []T
	idx     int
}

// newIterator constructs an Iterator from a page fetcher.
func newIterator[T any](fetch func(ctx context.Context, token string) (Page[T], error)) *Iterator[T] {
	return &Iterator[T]{fetch: fetch}
}

// Next advances the iterator. It returns false when there are no more items
// or when an error is set. Use Err to inspect errors.
func (it *Iterator[T]) Next(ctx context.Context) bool {
	if it.err != nil {
		return false
	}
	if it.idx < len(it.pending) {
		it.idx++
		return true
	}
	if it.done {
		return false
	}
	page, err := it.fetch(ctx, it.token)
	if err != nil {
		it.err = err
		return false
	}
	it.pending = page.Items
	it.token = page.NextPageToken
	it.done = page.NextPageToken == ""
	it.idx = 0
	return it.Next(ctx)
}

// Item returns the current item. Call Next first.
func (it *Iterator[T]) Item() T {
	var zero T
	if it.idx == 0 || it.idx > len(it.pending) {
		return zero
	}
	return it.pending[it.idx-1]
}

// Err returns the error that stopped iteration, if any.
func (it *Iterator[T]) Err() error {
	if errors.Is(it.err, ErrInvalidResponse) {
		return it.err
	}
	return it.err
}
