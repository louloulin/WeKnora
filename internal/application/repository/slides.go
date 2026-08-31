// Package repository — Build #44 Slide persistence layer.
//
// Cross-dialect raw SQL (sqlite + postgres + mysql). The MapID →
// TenantID guard prevents cross-tenant leaks even if the URL is guessed.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// slideDeckRepository implements SlideDeckRepository using GORM.
type slideDeckRepository struct {
	db *gorm.DB
}

// NewSlideDeckRepository wires the repository.
func NewSlideDeckRepository(db *gorm.DB) interfaces.SlideDeckRepository {
	return &slideDeckRepository{db: db}
}

// CreateDeck persists a new deck.
func (r *slideDeckRepository) CreateDeck(ctx context.Context, d *types.SlideDeck) error {
	if err := d.Validate(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(d).Error
}

// GetDeck fetches one deck scoped to (tenant, id).
func (r *slideDeckRepository) GetDeck(ctx context.Context, tenantID uint64, id string) (*types.SlideDeck, error) {
	var d types.SlideDeck
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&d).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("slide_deck get: %w", err)
	}
	return &d, nil
}

// UpdateDeck applies a partial patch.
func (r *slideDeckRepository) UpdateDeck(ctx context.Context, tenantID uint64, id string, patch types.UpdateSlideDeckRequest) (*types.SlideDeck, error) {
	updates := map[string]any{}
	if patch.Title != nil {
		updates["title"] = *patch.Title
	}
	if patch.Theme != nil {
		if !types.ValidSlideThemes[*patch.Theme] {
			return nil, types.ErrSlideInvalid("theme is invalid")
		}
		updates["theme"] = *patch.Theme
	}
	if patch.Visibility != nil {
		updates["visibility"] = *patch.Visibility
	}
	if len(updates) == 0 {
		return r.GetDeck(ctx, tenantID, id)
	}
	res := r.db.WithContext(ctx).
		Model(&types.SlideDeck{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Updates(updates)
	if res.Error != nil {
		return nil, fmt.Errorf("slide_deck update: %w", res.Error)
	}
	return r.GetDeck(ctx, tenantID, id)
}

// DeleteDeck removes the deck + all slides (transactional).
func (r *slideDeckRepository) DeleteDeck(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND deck_id = ?", tenantID, id).
			Delete(&types.Slide{}).Error; err != nil {
			return fmt.Errorf("slide_deck delete slides: %w", err)
		}
		res := tx.Where("tenant_id = ? AND id = ?", tenantID, id).
			Delete(&types.SlideDeck{})
		if res.Error != nil {
			return fmt.Errorf("slide_deck delete: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return types.ErrSlideInvalid("slide deck not found")
		}
		return nil
	})
}

// ListDecks returns decks with filters.
func (r *slideDeckRepository) ListDecks(ctx context.Context, tenantID uint64, filter types.ListSlideDecksFilter) ([]*types.SlideDeck, error) {
	q := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if filter.KBID != "" {
		q = q.Where("kb_id = ?", filter.KBID)
	}
	if filter.OwnerUserID != 0 {
		q = q.Where("owner_user_id = ?", filter.OwnerUserID)
	}
	if filter.Visibility != "" {
		q = q.Where("visibility = ?", filter.Visibility)
	}
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 100
	}
	q = q.Order("updated_at DESC").Limit(filter.Limit).Offset(filter.Offset)
	var rows []*types.SlideDeck
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("slide_deck list: %w", err)
	}
	return rows, nil
}

// CountDecks returns the count for the same filters.
func (r *slideDeckRepository) CountDecks(ctx context.Context, tenantID uint64, filter types.ListSlideDecksFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&types.SlideDeck{}).Where("tenant_id = ?", tenantID)
	if filter.KBID != "" {
		q = q.Where("kb_id = ?", filter.KBID)
	}
	if filter.OwnerUserID != 0 {
		q = q.Where("owner_user_id = ?", filter.OwnerUserID)
	}
	if filter.Visibility != "" {
		q = q.Where("visibility = ?", filter.Visibility)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, fmt.Errorf("slide_deck count: %w", err)
	}
	return n, nil
}

// CreateSlide persists a single slide.
func (r *slideDeckRepository) CreateSlide(ctx context.Context, s *types.Slide) error {
	if err := s.Validate(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(s).Error
}

// GetSlide fetches one slide scoped to (tenant, deck, slide).
func (r *slideDeckRepository) GetSlide(ctx context.Context, tenantID uint64, deckID, slideID string) (*types.Slide, error) {
	var s types.Slide
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND deck_id = ? AND id = ?", tenantID, deckID, slideID).
		First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("slide get: %w", err)
	}
	return &s, nil
}

// UpdateSlide applies a partial patch.
func (r *slideDeckRepository) UpdateSlide(ctx context.Context, tenantID uint64, deckID, slideID string, patch types.UpdateSlideRequest) (*types.Slide, error) {
	updates := map[string]any{}
	if patch.Layout != nil {
		if !types.ValidSlideLayouts[*patch.Layout] {
			return nil, types.ErrSlideInvalid("layout is invalid")
		}
		updates["layout"] = *patch.Layout
	}
	if patch.Title != nil {
		updates["title"] = *patch.Title
	}
	if patch.Body != nil {
		updates["body"] = *patch.Body
	}
	if patch.Bullets != nil {
		updates["bullets"] = marshalBullets(*patch.Bullets)
	}
	if patch.LeftCol != nil {
		updates["left_col"] = *patch.LeftCol
	}
	if patch.RightCol != nil {
		updates["right_col"] = *patch.RightCol
	}
	if patch.ImageURL != nil {
		updates["image_url"] = *patch.ImageURL
	}
	if patch.QuoteText != nil {
		updates["quote_text"] = *patch.QuoteText
	}
	if patch.QuoteAttr != nil {
		updates["quote_attr"] = *patch.QuoteAttr
	}
	if patch.Notes != nil {
		updates["notes"] = *patch.Notes
	}
	if patch.Background != nil {
		updates["background"] = *patch.Background
	}
	if len(updates) == 0 {
		return r.GetSlide(ctx, tenantID, deckID, slideID)
	}
	res := r.db.WithContext(ctx).
		Model(&types.Slide{}).
		Where("tenant_id = ? AND deck_id = ? AND id = ?", tenantID, deckID, slideID).
		Updates(updates)
	if res.Error != nil {
		return nil, fmt.Errorf("slide update: %w", res.Error)
	}
	return r.GetSlide(ctx, tenantID, deckID, slideID)
}

// DeleteSlide removes a single slide.
func (r *slideDeckRepository) DeleteSlide(ctx context.Context, tenantID uint64, deckID, slideID string) error {
	res := r.db.WithContext(ctx).
		Where("tenant_id = ? AND deck_id = ? AND id = ?", tenantID, deckID, slideID).
		Delete(&types.Slide{})
	if res.Error != nil {
		return fmt.Errorf("slide delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return types.ErrSlideInvalid("slide not found")
	}
	return nil
}

// ListSlidesByDeck returns every slide for a deck.
func (r *slideDeckRepository) ListSlidesByDeck(ctx context.Context, tenantID uint64, deckID string) ([]*types.Slide, error) {
	var rows []*types.Slide
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND deck_id = ?", tenantID, deckID).
		Order("`index` ASC, created_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("slide list: %w", err)
	}
	return rows, nil
}

// DeleteByKB removes all decks + slides for a KB.
func (r *slideDeckRepository) DeleteByKB(ctx context.Context, tenantID uint64, kbID string) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("tenant_id = ? AND deck_id IN (?)",
			tenantID,
			tx.Model(&types.SlideDeck{}).
				Select("id").
				Where("tenant_id = ? AND kb_id = ?", tenantID, kbID),
		).Delete(&types.Slide{})
		if res.Error != nil {
			return res.Error
		}
		total += res.RowsAffected
		res2 := tx.Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).
			Delete(&types.SlideDeck{})
		if res2.Error != nil {
			return res2.Error
		}
		total += res2.RowsAffected
		return nil
	})
	return total, err
}

// marshalBullets encodes []string to a JSON array. Empty slice → "[]".
func marshalBullets(items []string) string {
	if items == nil {
		return "[]"
	}
	b, _ := json.Marshal(items)
	return string(b)
}