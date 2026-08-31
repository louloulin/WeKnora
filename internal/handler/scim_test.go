package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/scimsp"
)

// TestSCIMErrorEnvelopeShape asserts the wire envelope matches RFC
// 7644 §3.7.3: schemas must include the error schema URI, status
// must be the HTTP code, scimType must be omitted when empty.
func TestSCIMErrorEnvelopeShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	writeSCIMError(c, http.StatusBadRequest, "bad filter", "invalidFilter")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
	if got := rec.Header().Get("Content-Type"); got != scimsp.ContentType {
		t.Fatalf("content type: got %q want %q", got, scimsp.ContentType)
	}
	body := rec.Body.String()
	if !strings.Contains(body, scimsp.SchemaError) {
		t.Fatalf("body missing error schema: %s", body)
	}
	if !strings.Contains(body, `"invalidFilter"`) {
		t.Fatalf("body missing scimType: %s", body)
	}
}

// TestSCIMErrorOmitsEmptyScimType makes sure we do not emit
// `"scimType":""` when the caller leaves it blank — IdPs treat
// the empty string as a real value and reject the response.
func TestSCIMErrorOmitsEmptyScimType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	writeSCIMError(c, http.StatusUnauthorized, "no auth", "")
	if strings.Contains(rec.Body.String(), `"scimType":""`) {
		t.Fatalf("scimType should be omitted when empty: %s", rec.Body.String())
	}
}

// TestSCIMMiddlewareRejectsMissingHeader drives the middleware
// path with a stub token service. We exercise only the failure arm
// because the success arm needs a fully wired userService.
func TestSCIMMiddlewareRejectsMissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// No auth header at all → 401 with SCIM error envelope.
	r.GET("/scim/v2/Users", SCIMMiddleware(nil), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scim/v2/Users", bytes.NewReader(nil))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), scimsp.SchemaError) {
		t.Fatalf("body not in scim error envelope: %s", rec.Body.String())
	}
}
