package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/ldapsp"
)

// TestRespondLDAPErrorMapping covers the service-layer error → HTTP
// status mapping without booting the full handler stack. Building a
// full handler needs the concrete *service.LDAPConfigService and the
// entire UserService interface, which is exercised by the
// integration suite.
func TestRespondLDAPErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"config not found", repository.ErrLDAPConfigNotFound, http.StatusNotFound},
		{"federation not found", service.ErrLDAPFederationNotFound, http.StatusNotFound},
		{"federation revoked", service.ErrLDAPFederationRevoked, http.StatusForbidden},
		{"invalid credentials (service)", service.ErrLDAPInvalidCredentials, http.StatusUnauthorized},
		{"entry not found", service.ErrLDAPEntryNotFound, http.StatusUnauthorized},
		{"missing email", service.ErrLDAPMissingEmail, http.StatusBadRequest},
		{"linking disabled", service.ErrLDAPIdentityLinkingDisabled, http.StatusForbidden},
		{"invalid credentials (sp)", ldapsp.ErrInvalidCredentials, http.StatusUnauthorized},
		{"unknown error", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			respondLDAPError(c, tc.err)
			if rec.Code != tc.status {
				t.Fatalf("status: got %d want %d body=%s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}
