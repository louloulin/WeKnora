package middleware

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service/region"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// RegionEnforcer injects the resolved region into the request context
// and blocks requests whose target region violates the tenant's
// residency policy. It also writes a cross-region audit entry for
// every cross-region action.
//
// Mount AFTER the auth middleware so that "tenant_id" / "user_id"
// context keys are populated.
func RegionEnforcer(svc *region.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := tenantIDFromCtx(c)
		if tenantID == 0 {
			// No tenant context — let the auth layer reject later.
			c.Next()
			return
		}

		ip := c.ClientIP()
		resolved, err := svc.Resolve(c, tenantID, ip)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "region_resolve_failed"})
			return
		}
		c.Set("region", resolved)

		// If the caller explicitly asked for a different region (rare —
		// admin tools, replication jobs), gate it.
		requested := c.GetHeader("X-Target-Region")
		if requested != "" && requested != string(resolved) {
			dst := types.Region(requested)
			if err := svc.ComplianceGate(c, tenantID, resolved, dst); err != nil {
				_ = svc.AuditCrossRegion(c, &types.CrossRegionAuditLog{
					SourceRegion: resolved,
					TargetRegion: dst,
					TenantID:     tenantID,
					UserID:       userIDFromCtx(c),
					Action:       types.CrossRegionActionRead,
					ResourceType: "request",
					ResourceID:   c.Request.URL.Path,
					Allowed:      false,
					Reason:       err.Error(),
				})
				c.AbortWithStatusJSON(http.StatusUnavailableForLegalReasons,
					gin.H{"error": "residency_violation"})
				return
			}
			_ = svc.AuditCrossRegion(c, &types.CrossRegionAuditLog{
				SourceRegion: resolved,
				TargetRegion: dst,
				TenantID:     tenantID,
				UserID:       userIDFromCtx(c),
				Action:       types.CrossRegionActionRead,
				ResourceType: "request",
				ResourceID:   c.Request.URL.Path,
				Allowed:      true,
			})
		}
		c.Next()
	}
}

// tenantIDFromCtx extracts the tenant_id context key set by the auth
// middleware. Returns 0 if absent (tenantless session).
func tenantIDFromCtx(c *gin.Context) uint64 {
	if v, ok := c.Get("tenant_id"); ok {
		if id, ok := v.(uint64); ok {
			return id
		}
		if id, ok := v.(float64); ok {
			return uint64(id)
		}
	}
	return 0
}

// userIDFromCtx extracts the user_id context key set by the auth
// middleware. Returns empty string if absent.
func userIDFromCtx(c *gin.Context) string {
	if v, ok := c.Get("user_id"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
