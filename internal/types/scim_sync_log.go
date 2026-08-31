package types

import (
	"time"

	"gorm.io/gorm"
)

// SCIMSyncLog records every SCIM operation that succeeded or
// failed. Used by the admin diagnostics view and by ops to spot
// misbehaving IdPs (constant 401s, repeated validation errors).
//
// One row per request, regardless of how many resources the
// request touched — payload size is bounded by the typical IdP
// (Okta pushes one user per request, Azure AD allows up to ~50).
type SCIMSyncLog struct {
	ID           uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID     uint64         `gorm:"not null;index" json:"tenant_id"`
	TokenID      uint64         `gorm:"not null;index" json:"token_id"`
	Method       string         `gorm:"size:10;not null" json:"method"`         // GET / POST / PUT / PATCH / DELETE
	Path         string         `gorm:"size:255;not null" json:"path"`          // e.g. /scim/v2/Users/abc-123
	ResourceType string         `gorm:"size:32" json:"resource_type,omitempty"` // User / Group / discovery
	Status       int            `gorm:"not null" json:"status"`                 // HTTP status returned
	Subject      string         `gorm:"size:255" json:"subject,omitempty"`      // userName or group displayName
	Detail       string         `gorm:"type:text" json:"detail,omitempty"`      // error message when non-2xx
	CreatedAt    time.Time      `gorm:"not null;index" json:"created_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName pins the table name.
func (SCIMSyncLog) TableName() string { return "scim_sync_logs" }
