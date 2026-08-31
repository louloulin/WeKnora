// Package interfaces holds the contract types used by the application
// layer to talk to persistence without binding to gorm directly.
package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// KGRepository is the storage contract for Supertags, Entities, and
// KGEntityRelations. The interface deliberately keeps query methods
// narrow so the gorm implementation can stay focused.
type KGRepository interface {
	// KGSupertag CRUD
	CreateSupertag(ctx context.Context, st *types.KGSupertag) error
	GetSupertag(ctx context.Context, tenantID uint64, id string) (*types.KGSupertag, error)
	ListSupertagsByKB(ctx context.Context, tenantID uint64, kbID string) ([]*types.KGSupertag, error)
	UpdateSupertag(ctx context.Context, st *types.KGSupertag) error
	DeleteSupertag(ctx context.Context, tenantID uint64, id string) error

	// KGEntity CRUD + lookup
	CreateEntity(ctx context.Context, e *types.KGEntity) error
	GetEntity(ctx context.Context, tenantID uint64, id string) (*types.KGEntity, error)
	FindEntitiesByName(ctx context.Context, tenantID uint64, kbID, name string) ([]*types.KGEntity, error)
	UpdateEntity(ctx context.Context, e *types.KGEntity) error
	BumpEntityOccurrence(ctx context.Context, id string) error
	ListEntitiesBySupertag(ctx context.Context, tenantID uint64, supertagID string, limit int) ([]*types.KGEntity, error)

	// Relation CRUD + queries
	CreateRelation(ctx context.Context, r *types.KGEntityRelation) error
	ListRelationsByEntity(ctx context.Context, tenantID uint64, entityID string, limit int) ([]*types.KGEntityRelation, error)
	ListRelationsByKB(ctx context.Context, tenantID uint64, kbID string, limit int) ([]*types.KGEntityRelation, error)

	// KGSupertag commands
	CreateKGSupertagCommand(ctx context.Context, cmd *types.KGSupertagCommand) error
	ListKGSupertagCommands(ctx context.Context, supertagID, event string) ([]*types.KGSupertagCommand, error)
}
