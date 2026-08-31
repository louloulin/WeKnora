// Package middleware — collab_ratelimit.go (v0.7.38 Build #46.x).
//
// Per-tenant / per-doc / per-IP rate limiters for the collaborative_docs
// surface. Without these a single tenant (or a single hot doc, e.g. a
// widely-shared meeting note) can saturate the WS upgrade / upload /
// snapshot endpoints and starve other tenants.
//
// Budgets are conservative defaults — a single user typing + autosave
// is well under 1 req/s on the WS path; the per-doc cap covers the case
// of 50 collaborators each on a 200ms debounce (≈250 req/min per doc).
// Override via env at process start if a tenant proves otherwise.
package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/ratelimit"
	"github.com/Tencent/WeKnora/internal/types"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	collabTenantRateLimitKeyPrefix = "collab:ratelimit:tenant:"
	collabDocRateLimitKeyPrefix    = "collab:ratelimit:doc:"
	collabIPRateLimitKeyPrefix     = "collab:ratelimit:ip:"

	// CollabTenantPerMin is the per-tenant per-minute cap. A tenant with
	// 200 active collaborators all autosaving 1Hz easily fits under this;
	// the cap mainly blocks bulk-burst from a single misbehaving client.
	collabTenantPerMin = 600
	// CollabDocPerMin bounds a single hot doc from absorbing the entire
	// tenant budget (50 collaborators × ~1 req/s edit rate).
	collabDocPerMin = 300
	// CollabIPPerMin is a fallback when the request has no tenant or doc
	// context yet (e.g. POST /collaborative-docs before doc creation).
	collabIPPerMin = 120

	collabRateLimitWindow = 60 * time.Second
)

var (
	collabTenantLimiterOnce sync.Once
	collabTenantLimiter     *ratelimit.Limiter
	collabDocLimiterOnce    sync.Once
	collabDocLimiter        *ratelimit.Limiter
	collabIPLimiterOnce     sync.Once
	collabIPLimiter         *ratelimit.Limiter
)

func collabTenantLimiterSingleton(redisClient *redis.Client) *ratelimit.Limiter {
	collabTenantLimiterOnce.Do(func() {
		collabTenantLimiter = ratelimit.New(redisClient, collabTenantRateLimitKeyPrefix, collabRateLimitWindow, "")
		stopCh := make(chan struct{})
		go collabTenantLimiter.StartCleanup(stopCh)
	})
	return collabTenantLimiter
}

func collabDocLimiterSingleton(redisClient *redis.Client) *ratelimit.Limiter {
	collabDocLimiterOnce.Do(func() {
		collabDocLimiter = ratelimit.New(redisClient, collabDocRateLimitKeyPrefix, collabRateLimitWindow, "")
		stopCh := make(chan struct{})
		go collabDocLimiter.StartCleanup(stopCh)
	})
	return collabDocLimiter
}

func collabIPLimiterSingleton(redisClient *redis.Client) *ratelimit.Limiter {
	collabIPLimiterOnce.Do(func() {
		collabIPLimiter = ratelimit.New(redisClient, collabIPRateLimitKeyPrefix, collabRateLimitWindow, "")
		stopCh := make(chan struct{})
		go collabIPLimiter.StartCleanup(stopCh)
	})
	return collabIPLimiter
}

// CollabTenantRateLimit enforces per-tenant budget on the collab surface.
// Skips when the request carries no tenant (e.g. an unauthenticated share
// link — that's covered by PublicAuthRateLimit elsewhere).
func CollabTenantRateLimit(redisClient *redis.Client) gin.HandlerFunc {
	limiter := collabTenantLimiterSingleton(redisClient)
	return func(c *gin.Context) {
		tenantID, ok := types.TenantIDFromContext(c.Request.Context())
		if !ok || tenantID == 0 {
			c.Next()
			return
		}
		key := strconv.FormatUint(tenantID, 10)
		if !limiter.Allow(c.Request.Context(), key, collabTenantPerMin) {
			respondRateLimited(c, "tenant", key, collabTenantPerMin)
			return
		}
		c.Next()
	}
}

// CollabDocRateLimit enforces per-doc budget. The doc id is sourced from
// the :id URL parameter; routes that have no :id (list / create) skip.
func CollabDocRateLimit(redisClient *redis.Client) gin.HandlerFunc {
	limiter := collabDocLimiterSingleton(redisClient)
	return func(c *gin.Context) {
		docID := c.Param("id")
		if docID == "" {
			c.Next()
			return
		}
		if !limiter.Allow(c.Request.Context(), docID, collabDocPerMin) {
			respondRateLimited(c, "doc", docID, collabDocPerMin)
			return
		}
		c.Next()
	}
}

// CollabIPRateLimit is the fallback for endpoints with no tenant / doc
// context. Caps unauthenticated and pre-create traffic.
func CollabIPRateLimit(redisClient *redis.Client) gin.HandlerFunc {
	limiter := collabIPLimiterSingleton(redisClient)
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.Allow(c.Request.Context(), ip, collabIPPerMin) {
			respondRateLimited(c, "ip", ip, collabIPPerMin)
			return
		}
		c.Next()
	}
}

func respondRateLimited(c *gin.Context, scope, key string, max int) {
	logger.Warnf(c.Request.Context(),
		"[collab-ratelimit] denied scope=%s key=%s max=%d", scope, key, max)
	c.Header("Retry-After", "5")
	c.Error(&apperrors.AppError{
		Code:     apperrors.ErrTooManyRequests,
		Message:  "collaborative_docs rate limit exceeded; retry shortly",
		HTTPCode: http.StatusTooManyRequests,
	})
	c.Abort()
}
