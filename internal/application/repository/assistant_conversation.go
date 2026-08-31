package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type assistantConversationRepository struct {
	db *gorm.DB
}

// NewAssistantConversationRepository wires the GORM implementation
// into the DI container.
func NewAssistantConversationRepository(db *gorm.DB) interfaces.AssistantConversationRepository {
	return &assistantConversationRepository{db: db}
}

func (r *assistantConversationRepository) Create(ctx context.Context, c *types.AssistantConversation) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *assistantConversationRepository) ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*types.AssistantConversation, int64, error) {
	q := r.db.WithContext(ctx).Model(&types.AssistantConversation{}).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("created_at DESC")
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	var rows []*types.AssistantConversation
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *assistantConversationRepository) ListByConversation(ctx context.Context, conversationID string) ([]*types.AssistantConversation, error) {
	var rows []*types.AssistantConversation
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND deleted_at IS NULL", conversationID).
		Order("created_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
