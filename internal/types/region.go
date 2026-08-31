package types

import (
	"time"
)

// Region identifies a logical deployment region for multi-region
// support. The string format follows cloud-provider conventions
// ("us-east-1", "eu-central-1", "ap-northeast-1") so operators can
// map directly to their infra.
type Region string

const (
	RegionUSEast    Region = "us-east-1"
	RegionUSWest    Region = "us-west-2"
	RegionEU        Region = "eu-central-1" // Frankfurt
	RegionAPAC      Region = "ap-northeast-1" // Tokyo
	RegionDev       Region = "dev-local"
)

// AllRegions lists the supported regions for tenant binding.
var AllRegions = []Region{
	RegionUSEast,
	RegionUSWest,
	RegionEU,
	RegionAPAC,
	RegionDev,
}

// IsValid returns whether the region string is one of the registered regions.
func (r Region) IsValid() bool {
	for _, v := range AllRegions {
		if v == r {
			return true
		}
	}
	return false
}

// DataResidencyPolicy controls where tenant data is permitted to live.
// strict_local means data must never leave its primary region;
// eu_only / apac_only constrain to a single legal jurisdiction;
// global permits cross-region replication.
type DataResidencyPolicy string

const (
	// ResidencyStrictLocal: data must never leave the tenant's primary region.
	ResidencyStrictLocal DataResidencyPolicy = "strict_local"
	// ResidencyEUOnly: data must stay within EU jurisdictions.
	ResidencyEUOnly DataResidencyPolicy = "eu_only"
	// ResidencyAPACOnly: data must stay within APAC jurisdictions.
	ResidencyAPACOnly DataResidencyPolicy = "apac_only"
	// ResidencyGlobal: data may be replicated to any region.
	ResidencyGlobal DataResidencyPolicy = "global"
)

// AllResidencyPolicies lists the registered residency policies.
var AllResidencyPolicies = []DataResidencyPolicy{
	ResidencyStrictLocal,
	ResidencyEUOnly,
	ResidencyAPACOnly,
	ResidencyGlobal,
}

// IsValid returns whether the policy string is one of the registered policies.
func (p DataResidencyPolicy) IsValid() bool {
	for _, v := range AllResidencyPolicies {
		if v == p {
			return true
		}
	}
	return false
}

// AllowsCrossRegion returns whether the policy permits data to flow
// from src to dst. Strict policies block anything that crosses a
// region boundary.
func (p DataResidencyPolicy) AllowsCrossRegion(src, dst Region) bool {
	if src == dst {
		return true
	}
	switch p {
	case ResidencyStrictLocal:
		return false
	case ResidencyEUOnly:
		return isEURegion(dst)
	case ResidencyAPACOnly:
		return isAPACRegion(dst)
	case ResidencyGlobal:
		return true
	}
	return false
}

func isEURegion(r Region) bool {
	return r == RegionEU
}

func isAPACRegion(r Region) bool {
	return r == RegionAPAC
}

// TenantRegionBinding associates a tenant with its primary region,
// residency policy, and optional replica regions.
type TenantRegionBinding struct {
	TenantID        uint64             `json:"tenant_id" gorm:"primaryKey;autoIncrement"`
	PrimaryRegion   Region             `json:"primary_region" gorm:"type:varchar(32);index"`
	ResidencyPolicy DataResidencyPolicy `json:"residency_policy" gorm:"type:varchar(32);default:'strict_local'"`
	ReplicaRegions  []Region           `json:"replica_regions" gorm:"type:json"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// TableName tells GORM to use tenant_region_bindings table.
func (TenantRegionBinding) TableName() string { return "tenant_region_bindings" }

// RegionStatus enumerates the lifecycle of a Region record.
type RegionStatus string

const (
	RegionStatusActive    RegionStatus = "active"
	RegionStatusDegraded  RegionStatus = "degraded"
	RegionStatusDraining  RegionStatus = "draining"
	RegionStatusOffline   RegionStatus = "offline"
)

// RegionRecord describes one logical deployment region. Capacity and
// health are advisory fields populated by the runtime.
type RegionRecord struct {
	ID          string       `json:"id" gorm:"primaryKey;type:varchar(32)"`
	DisplayName string       `json:"display_name" gorm:"type:varchar(64)"`
	Location    string       `json:"location" gorm:"type:varchar(64)"`
	Status      RegionStatus `json:"status" gorm:"type:varchar(16);default:'active'"`
	CapacityPct int          `json:"capacity_pct"`
	LatencyMs   int          `json:"latency_ms"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// TableName tells GORM to use regions table.
func (RegionRecord) TableName() string { return "regions" }

// CrossRegionAuditAction enumerates the kinds of cross-region
// operations that are recorded for compliance review.
type CrossRegionAuditAction string

const (
	CrossRegionActionRead      CrossRegionAuditAction = "read"
	CrossRegionActionWrite     CrossRegionAuditAction = "write"
	CrossRegionActionReplicate CrossRegionAuditAction = "replicate"
	CrossRegionActionAdmin     CrossRegionAuditAction = "admin"
)

// CrossRegionAuditLog records one cross-region action. Used by the
// Governance dashboard and the RegionEnforcer middleware.
type CrossRegionAuditLog struct {
	ID           uint64               `json:"id" gorm:"primaryKey;autoIncrement"`
	SourceRegion Region               `json:"source_region" gorm:"type:varchar(32);index"`
	TargetRegion Region               `json:"target_region" gorm:"type:varchar(32);index"`
	TenantID     uint64               `json:"tenant_id" gorm:"index"`
	UserID       string               `json:"user_id" gorm:"type:varchar(64)"`
	Action       CrossRegionAuditAction `json:"action" gorm:"type:varchar(16);index"`
	ResourceType string               `json:"resource_type" gorm:"type:varchar(64)"`
	ResourceID   string               `json:"resource_id" gorm:"type:varchar(128)"`
	Allowed      bool                 `json:"allowed" gorm:"index"`
	Reason       string               `json:"reason" gorm:"type:text"`
	Timestamp    time.Time            `json:"timestamp" gorm:"index"`
}

// TableName tells GORM to use cross_region_audit_log table.
func (CrossRegionAuditLog) TableName() string { return "cross_region_audit_log" }
