// Package interfaces — Slide deck / slide repository contract.
package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// SlideDeckRepository persists SlideDeck + Slide aggregates.
type SlideDeckRepository interface {
	CreateDeck(ctx context.Context, d *types.SlideDeck) error
	GetDeck(ctx context.Context, tenantID uint64, id string) (*types.SlideDeck, error)
	UpdateDeck(ctx context.Context, tenantID uint64, id string, patch types.UpdateSlideDeckRequest) (*types.SlideDeck, error)
	DeleteDeck(ctx context.Context, tenantID uint64, id string) error
	ListDecks(ctx context.Context, tenantID uint64, filter types.ListSlideDecksFilter) ([]*types.SlideDeck, error)
	CountDecks(ctx context.Context, tenantID uint64, filter types.ListSlideDecksFilter) (int64, error)

	CreateSlide(ctx context.Context, s *types.Slide) error
	GetSlide(ctx context.Context, tenantID uint64, deckID, slideID string) (*types.Slide, error)
	UpdateSlide(ctx context.Context, tenantID uint64, deckID, slideID string, patch types.UpdateSlideRequest) (*types.Slide, error)
	DeleteSlide(ctx context.Context, tenantID uint64, deckID, slideID string) error
	ListSlidesByDeck(ctx context.Context, tenantID uint64, deckID string) ([]*types.Slide, error)

	DeleteByKB(ctx context.Context, tenantID uint64, kbID string) (int64, error)
}
