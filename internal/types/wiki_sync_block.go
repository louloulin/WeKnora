package types

import (
	"encoding/json"
	"fmt"
	"time"
)

// WikiSyncBlock is the canonical source for a synced block — the reusable
// content unit that lives independently of any specific page. Pages embed
// references to it via wiki_sync_block_refs.
//
// The model mirrors Notion Synced Blocks / 飞书同步块 / Microsoft Loop
// components: edit the canonical once, every embedded reference re-renders
// to the latest version automatically.
type WikiSyncBlock struct {
	ID          uint64          `json:"id"`
	TenantID    uint64          `json:"tenant_id" gorm:"index"`
	KBID        string          `json:"kb_id" gorm:"type:varchar(36);index"`
	BlockID     string          `json:"block_id" gorm:"type:varchar(36);index"`
	Title       string          `json:"title" gorm:"type:varchar(256)"`
	ContentJSON string `json:"content_json" gorm:"column:content_json;type:text"`
	ContentMD   string          `json:"content_md" gorm:"column:content_md;type:text"`
	Version     int64           `json:"version" gorm:"default:1"`
	OwnerID     uint64          `json:"owner_id" gorm:"index"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// RawJSON returns ContentJSON as a json.RawMessage for handler-layer use.
func (s *WikiSyncBlock) RawJSON() json.RawMessage {
	if s.ContentJSON == "" {
		return json.RawMessage("{}")
	}
	return json.RawMessage(s.ContentJSON)
}

// TableName tells GORM to use the wiki_sync_blocks table for this model.
func (WikiSyncBlock) TableName() string { return "wiki_sync_blocks" }

// WikiSyncBlockRef is one embedded reference to a synced block. When the
// canonical block changes, every ref is touched (rendered_at advanced,
// content_version updated) so pages know to re-render.
type WikiSyncBlockRef struct {
	ID             uint64    `json:"id"`
	TenantID       uint64    `json:"tenant_id" gorm:"index"`
	KBID           string    `json:"kb_id" gorm:"type:varchar(36);index"`
	BlockID        string    `json:"block_id" gorm:"type:varchar(36);index"`
	PageID         string    `json:"page_id" gorm:"type:varchar(36);index"`
	AnchorSlug     string    `json:"anchor_slug" gorm:"type:varchar(256)"`
	ContentVersion int64     `json:"content_version" gorm:"default:0"`
	RenderedAt     time.Time `json:"rendered_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// TableName tells GORM to use the wiki_sync_block_refs table for this model.
func (WikiSyncBlockRef) TableName() string { return "wiki_sync_block_refs" }

// WikiSyncBlockUpsert is the input shape for creating or updating a
// canonical synced block. The service layer validates and dispatches.
type WikiSyncBlockUpsert struct {
	TenantID    uint64
	KBID        string
	BlockID     string
	Title       string
	ContentJSON json.RawMessage
	ContentMD   string
	OwnerID     uint64
}

// Validate enforces the non-empty invariants.
func (u WikiSyncBlockUpsert) Validate() error {
	if u.TenantID == 0 {
		return ErrSyncBlockInvalid("tenant_id is required")
	}
	if u.KBID == "" {
		return ErrSyncBlockInvalid("kb_id is required")
	}
	if u.BlockID == "" {
		return ErrSyncBlockInvalid("block_id is required")
	}
	if len(u.ContentJSON) == 0 {
		return ErrSyncBlockInvalid("content_json is required")
	}
	if u.OwnerID == 0 {
		return ErrSyncBlockInvalid("owner_id is required")
	}
	return nil
}

// ErrSyncBlockInvalid is the typed validation error used by the package.
type ErrSyncBlockInvalid string

func (e ErrSyncBlockInvalid) Error() string {
	return fmt.Sprintf("wiki sync block invalid: %s", string(e))
}

// WikiSyncBlockRefStats summarizes fan-out reach for a single block.
type WikiSyncBlockRefStats struct {
	BlockID         string `json:"block_id"`
	RefCount        int64  `json:"ref_count"`
	PagesCount      int64  `json:"pages_count"`
	StaleRefCount   int64  `json:"stale_ref_count"` // refs where content_version < canonical version
	CurrentVersion  int64  `json:"current_version"`
}
