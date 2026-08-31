package types

import (
	"encoding/json"
	"time"
)

// PluginTrustTier enumerates the editorial review tier a plugin has
// passed. Mirrors Atlassian's Cloud Fortified / Enterprise Certified
// programs: stricter review → more enterprise trust → broader
// install surface.
type PluginTrustTier string

const (
	// PluginTrustBasic: passed automated checks, not manually reviewed.
	PluginTrustBasic PluginTrustTier = "basic"
	// PluginTrustFortified: passed the security + reliability + support bar.
	PluginTrustFortified PluginTrustTier = "fortified"
	// PluginTrustEnterprise: passed the enterprise trust bar (SOC 2, pen test).
	PluginTrustEnterprise PluginTrustTier = "enterprise"
)

// AllPluginTrustTiers lists the registered trust tiers.
var AllPluginTrustTiers = []PluginTrustTier{
	PluginTrustBasic,
	PluginTrustFortified,
	PluginTrustEnterprise,
}

// IsValid returns whether the tier string is one of the registered tiers.
func (t PluginTrustTier) IsValid() bool {
	for _, v := range AllPluginTrustTiers {
		if v == t {
			return true
		}
	}
	return false
}

// PluginReviewStatus enumerates the marketplace editorial pipeline.
// Plugins flow Draft → Submitted → Approved → Published. Rejected
// or Disabled are terminal until a new version is uploaded.
type PluginReviewStatus string

const (
	PluginReviewDraft     PluginReviewStatus = "draft"
	PluginReviewSubmitted PluginReviewStatus = "submitted"
	PluginReviewApproved  PluginReviewStatus = "approved"
	PluginReviewPublished PluginReviewStatus = "published"
	PluginReviewRejected  PluginReviewStatus = "rejected"
	PluginReviewDisabled  PluginReviewStatus = "disabled"
)

// AllPluginReviewStatuses lists the registered review statuses.
var AllPluginReviewStatuses = []PluginReviewStatus{
	PluginReviewDraft,
	PluginReviewSubmitted,
	PluginReviewApproved,
	PluginReviewPublished,
	PluginReviewRejected,
	PluginReviewDisabled,
}

// IsValid returns whether the status string is one of the registered statuses.
func (s PluginReviewStatus) IsValid() bool {
	for _, v := range AllPluginReviewStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// IsPublic returns whether the plugin should appear in the public
// marketplace catalog (i.e. it has passed review and is live).
func (s PluginReviewStatus) IsPublic() bool {
	return s == PluginReviewPublished
}

// PluginManifest is the author-supplied metadata for a single plugin
// version. Author signs CanonicalBytes() with their RSA-PSS key;
// the marketplace rejects manifests whose signature does not verify
// against the author's registered public key.
type PluginManifest struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	Description      string   `json:"description"`
	Author           string   `json:"author"`
	AuthorPublicKey  string   `json:"author_public_key"` // PEM-encoded RSA public key
	Signature        string   `json:"signature"`         // base64 RSA-PSS signature over CanonicalBytes()
	Homepage         string   `json:"homepage,omitempty"`
	Permissions      []string `json:"permissions"`
	MinWeKnoraVersion string  `json:"min_weknora_version"`
	EntryPoint       string   `json:"entry_point"` // path inside the artifact tarball
	ArtifactSHA256   string   `json:"artifact_sha256"`
	IconURL          string   `json:"icon_url,omitempty"`
}

// CanonicalBytes returns the deterministic byte representation that
// gets signed. Excludes the Signature field itself to avoid a
// recursive dependency.
func (m *PluginManifest) CanonicalBytes() ([]byte, error) {
	// Strip signature so the same bytes always get signed.
	clone := *m
	clone.Signature = ""
	return json.Marshal(&clone)
}

// GetAuthorPublicKey satisfies the marketplace.SignatureInput
// interface without importing this package from there.
func (m *PluginManifest) GetAuthorPublicKey() string { return m.AuthorPublicKey }

// GetSignature satisfies the marketplace.SignatureInput interface.
func (m *PluginManifest) GetSignature() string { return m.Signature }

// PluginRecord is the marketplace-side row for one plugin version.
// Distinct from the manifest so the marketplace can add editorial
// fields (status, trust tier, downloads) without touching the
// author-controlled manifest.
type PluginRecord struct {
	ID            uint64             `json:"id" gorm:"primaryKey;autoIncrement"`
	PluginID      string             `json:"plugin_id" gorm:"type:varchar(64);index;uniqueIndex:uq_plugin_version"`
	Version       string             `json:"version" gorm:"type:varchar(32);uniqueIndex:uq_plugin_version"`
	Name          string             `json:"name" gorm:"type:varchar(128)"`
	Description   string             `json:"description" gorm:"type:text"`
	Author        string             `json:"author" gorm:"type:varchar(64);index"`
	Homepage      string             `json:"homepage" gorm:"type:varchar(255)"`
	Permissions   []string           `json:"permissions" gorm:"type:json"`
	EntryPoint    string             `json:"entry_point" gorm:"type:varchar(255)"`
	ArtifactURL   string             `json:"artifact_url" gorm:"type:varchar(255)"`
	ArtifactSHA256 string            `json:"artifact_sha256" gorm:"type:varchar(64)"`
	IconURL       string             `json:"icon_url" gorm:"type:varchar(255)"`
	Status        PluginReviewStatus `json:"status" gorm:"type:varchar(16);index"`
	TrustTier     PluginTrustTier    `json:"trust_tier" gorm:"type:varchar(16);index"`
	Downloads     int                `json:"downloads"`
	SubmittedAt   time.Time          `json:"submitted_at"`
	ReviewedAt    *time.Time         `json:"reviewed_at,omitempty"`
	ReviewerNote  string             `json:"reviewer_note" gorm:"type:text"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

// TableName tells GORM to use plugins table.
func (PluginRecord) TableName() string { return "plugins" }

// TenantPlugin records a tenant's install of a plugin. The
// Permissions field is the resolved permission set at install time
// (subset of the manifest permissions, possibly narrowed by tenant
// admin policy).
type TenantPlugin struct {
	ID          uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID    uint64    `json:"tenant_id" gorm:"index;uniqueIndex:uq_tenant_plugin"`
	PluginID    string    `json:"plugin_id" gorm:"type:varchar(64);uniqueIndex:uq_tenant_plugin"`
	Version     string    `json:"version" gorm:"type:varchar(32)"`
	Enabled     bool      `json:"enabled" gorm:"default:true"`
	Permissions []string  `json:"permissions" gorm:"type:json"`
	InstalledBy string    `json:"installed_by" gorm:"type:varchar(64)"`
	InstalledAt time.Time `json:"installed_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName tells GORM to use tenant_plugins table.
func (TenantPlugin) TableName() string { return "tenant_plugins" }

// PluginVendor stores the author identity + their registered
// public key. New plugin versions are signed with the matching
// private key; the marketplace verifies the signature against the
// vendor's stored public key.
type PluginVendor struct {
	ID            uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	Slug          string    `json:"slug" gorm:"type:varchar(64);uniqueIndex"`
	Name          string    `json:"name" gorm:"type:varchar(128)"`
	PublicKey     string    `json:"public_key" gorm:"type:text"` // PEM-encoded RSA public key
	ContactEmail  string    `json:"contact_email" gorm:"type:varchar(128)"`
	Verified      bool      `json:"verified"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName tells GORM to use plugin_vendors table.
func (PluginVendor) TableName() string { return "plugin_vendors" }

// PluginAuditAction enumerates the kinds of marketplace actions
// captured in the audit log (publish, approve, install, uninstall).
type PluginAuditAction string

const (
	PluginAuditPublish    PluginAuditAction = "publish"
	PluginAuditReview     PluginAuditAction = "review"
	PluginAuditInstall    PluginAuditAction = "install"
	PluginAuditUninstall  PluginAuditAction = "uninstall"
	PluginAuditDisable    PluginAuditAction = "disable"
	PluginAuditReject     PluginAuditAction = "reject"
)

// PluginAuditLog records every marketplace event. Used for
// compliance audits and dispute resolution.
type PluginAuditLog struct {
	ID         uint64            `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID   uint64            `json:"tenant_id" gorm:"index"`
	ActorID    string            `json:"actor_id" gorm:"type:varchar(64)"`
	Action     PluginAuditAction `json:"action" gorm:"type:varchar(16);index"`
	PluginID   string            `json:"plugin_id" gorm:"type:varchar(64);index"`
	Version    string            `json:"version" gorm:"type:varchar(32)"`
	Detail     string            `json:"detail" gorm:"type:text"`
	Timestamp  time.Time         `json:"timestamp" gorm:"index"`
}

// TableName tells GORM to use plugin_audit_log table.
func (PluginAuditLog) TableName() string { return "plugin_audit_log" }
