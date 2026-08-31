package kg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// Sentinel errors surfaced by the KGSupertag service.
var (
	ErrSupertagNotFound = errors.New("kg: supertag not found")
	ErrEntityNotFound   = errors.New("kg: entity not found")
)

// KGSupertagService is the entry point for KGSupertag CRUD + binding logic.
type KGSupertagService struct {
	repo interfaces.KGRepository
	now  func() time.Time
}

// NewKGSupertagService constructs a KGSupertagService.
func NewKGSupertagService(repo interfaces.KGRepository) *KGSupertagService {
	return &KGSupertagService{repo: repo, now: time.Now}
}

// SetNow overrides the wall clock for tests.
func (s *KGSupertagService) SetNow(now func() time.Time) { s.now = now }

// Create validates the JSON Schema payload and persists a new KGSupertag.
func (s *KGSupertagService) Create(ctx context.Context, st *types.KGSupertag) error {
	if st.Name == "" {
		return errors.New("kg: supertag name is required")
	}
	if st.Schema == nil {
		st.Schema = json.RawMessage("[]")
	}
	// Validate the schema parses.
	var fields []types.KGSupertagField
	if err := json.Unmarshal(st.Schema, &fields); err != nil {
		return fmt.Errorf("kg: supertag schema: %w", err)
	}
	if st.ID == "" {
		st.ID = uuid.NewString()
	}
	now := s.now()
	st.CreatedAt = now
	st.UpdatedAt = now
	return s.repo.CreateSupertag(ctx, st)
}

// Get returns a single KGSupertag by ID.
func (s *KGSupertagService) Get(ctx context.Context, tenantID uint64, id string) (*types.KGSupertag, error) {
	st, err := s.repo.GetSupertag(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, ErrSupertagNotFound
	}
	return st, nil
}

// ListByKB returns every KGSupertag belonging to a knowledge base.
func (s *KGSupertagService) ListByKB(ctx context.Context, tenantID uint64, kbID string) ([]*types.KGSupertag, error) {
	return s.repo.ListSupertagsByKB(ctx, tenantID, kbID)
}

// Update mutates an existing KGSupertag (the schema is re-validated).
func (s *KGSupertagService) Update(ctx context.Context, st *types.KGSupertag) error {
	if st.Schema != nil {
		var fields []types.KGSupertagField
		if err := json.Unmarshal(st.Schema, &fields); err != nil {
			return fmt.Errorf("kg: supertag schema: %w", err)
		}
	}
	st.UpdatedAt = s.now()
	return s.repo.UpdateSupertag(ctx, st)
}

// Delete removes a KGSupertag by ID.
func (s *KGSupertagService) Delete(ctx context.Context, tenantID uint64, id string) error {
	return s.repo.DeleteSupertag(ctx, tenantID, id)
}

// BindKGSupertag attaches a KGSupertag to an KGEntity, validating that the
// entity's properties satisfy the Supertag's required fields. If the
// entity already exists, the new tag replaces the previous one (Tana
// allows single-supertag per node by default).
func (s *KGSupertagService) BindSupertag(ctx context.Context, tenantID uint64, entityID, supertagID string, properties map[string]any) (*types.KGEntity, error) {
	entity, err := s.repo.GetEntity(ctx, tenantID, entityID)
	if err != nil || entity == nil {
		return nil, ErrEntityNotFound
	}
	tag, err := s.repo.GetSupertag(ctx, tenantID, supertagID)
	if err != nil || tag == nil {
		return nil, ErrSupertagNotFound
	}
	var fields []types.KGSupertagField
	_ = json.Unmarshal(tag.Schema, &fields)
	for _, f := range fields {
		if f.Required {
			if _, ok := properties[f.Name]; !ok {
				return nil, fmt.Errorf("kg: required field %q missing", f.Name)
			}
		}
	}
	entity.SupertagID = &supertagID
	if properties != nil {
		props, _ := json.Marshal(properties)
		entity.Properties = props
	}
	entity.UpdatedAt = s.now()
	if err := s.repo.UpdateEntity(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}
