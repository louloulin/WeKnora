package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// ctxWithTenant mirrors the auth middleware: it attaches a tenant id
// to a request context so TenantIDFromContext can read it back.
func ctxWithTenant(parent context.Context, tenantID uint64) context.Context {
	return context.WithValue(parent, types.TenantIDContextKey, tenantID)
}

func newCtxWith(tenantID uint64, docParam string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := ctxWithTenant(req.Context(), tenantID)
	c.Request = req.WithContext(ctx)
	if docParam != "" {
		c.Params = gin.Params{{Key: "id", Value: docParam}}
	}
	return c, w
}

func TestCollabTenantRateLimit_PassesFirstRequest(t *testing.T) {
	c, _ := newCtxWith(uniqueTenant(t), "")
	mw := CollabTenantRateLimit(nil)
	mw(c)
	if c.IsAborted() {
		t.Fatalf("first request should not be limited")
	}
	if len(c.Errors) != 0 {
		t.Fatalf("unexpected errors on first request: %v", c.Errors)
	}
}

func TestCollabTenantRateLimit_NoTenantSkips(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request = req
	mw := CollabTenantRateLimit(nil)
	mw(c)
	if c.IsAborted() {
		t.Fatalf("middleware should skip when no tenant in ctx")
	}
}

func TestCollabTenantRateLimit_BudgetEnforced(t *testing.T) {
	tenant := uniqueTenant(t)
	mw := CollabTenantRateLimit(nil)
	allowed, denied := 0, 0
	for i := 0; i < collabTenantPerMin+5; i++ {
		c, _ := newCtxWith(tenant, "")
		mw(c)
		if c.IsAborted() {
			denied++
		} else {
			allowed++
		}
	}
	if allowed != collabTenantPerMin {
		t.Fatalf("expected exactly %d allowed before deny; got allowed=%d denied=%d",
			collabTenantPerMin, allowed, denied)
	}
	if denied != 5 {
		t.Fatalf("expected 5 denied; got %d", denied)
	}
}

func TestCollabDocRateLimit_NoIDSkips(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/collaborative-docs", nil)
	c.Request = req
	c.Params = nil // no :id param
	mw := CollabDocRateLimit(nil)
	mw(c)
	if c.IsAborted() {
		t.Fatalf("middleware should skip when :id param absent")
	}
}

func TestCollabDocRateLimit_BudgetEnforced(t *testing.T) {
	docID := "doc-" + uniqueTenantAsString(t)
	mw := CollabDocRateLimit(nil)
	allowed, denied := 0, 0
	for i := 0; i < collabDocPerMin+3; i++ {
		c, _ := newCtxWith(uniqueTenant(t), docID)
		mw(c)
		if c.IsAborted() {
			denied++
		} else {
			allowed++
		}
	}
	if allowed != collabDocPerMin {
		t.Fatalf("expected %d allowed; got %d", collabDocPerMin, allowed)
	}
	if denied != 3 {
		t.Fatalf("expected 3 denied; got %d", denied)
	}
}

func TestCollabDocRateLimit_DocScopeIndependent(t *testing.T) {
	mw := CollabDocRateLimit(nil)
	docA := "docA-" + uniqueTenantAsString(t)
	docB := "docB-" + uniqueTenantAsString(t)

	// Saturate docA.
	for i := 0; i < collabDocPerMin; i++ {
		c, _ := newCtxWith(uniqueTenant(t), docA)
		mw(c)
	}
	// docB must still have full budget.
	c, _ := newCtxWith(uniqueTenant(t), docB)
	mw(c)
	if c.IsAborted() {
		t.Fatalf("docB request must not be limited by docA's budget")
	}
}

func TestCollabIPRateLimit_PassesFirstRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request = req
	mw := CollabIPRateLimit(nil)
	mw(c)
	if c.IsAborted() {
		t.Fatalf("first IP request should not be limited")
	}
}

// uniqueTenant returns a uint64 derived from the test name so each
// subtest gets its own limiter bucket even when running in parallel.
func uniqueTenant(t *testing.T) uint64 {
	t.Helper()
	h := uint64(1469598103934665603)
	for _, c := range t.Name() {
		h = h*1099511628211 ^ uint64(c)
	}
	if h == 0 {
		h = 1
	}
	return h
}

func uniqueTenantAsString(t *testing.T) string {
	return strconv.FormatUint(uniqueTenant(t), 10)
}
