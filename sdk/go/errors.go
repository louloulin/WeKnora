// Package weknora provides the official Go SDK for the WeKnora Enterprise
// Knowledge Platform API.
//
// The SDK exposes typed service surfaces for knowledge bases, RAG search,
// chat, multi-dim databases, formulas, automations, collaborative documents,
// the agent studio, AI connectors, and AI verification. It mirrors the
// structure of the OpenAPI specification at internal/openapi/spec.yaml.
package weknora

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors for control flow.
var (
	ErrUnauthorized    = errors.New("weknora: unauthorized")
	ErrForbidden       = errors.New("weknora: forbidden")
	ErrNotFound        = errors.New("weknora: not found")
	ErrRateLimited     = errors.New("weknora: rate limited")
	ErrServer          = errors.New("weknora: server error")
	ErrInvalidResponse = errors.New("weknora: invalid response")
)

// APIError is the structured error returned by the WeKnora REST API. It
// implements the error interface and exposes the HTTP status code so
// callers can branch on transport-level conditions.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

// Error renders the APIError as a string.
func (e *APIError) Error() string {
	return fmt.Sprintf("weknora: api error %d %q: %s", e.StatusCode, e.Code, e.Message)
}

// Is supports errors.Is comparisons against the sentinel errors above.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrUnauthorized:
		return e.StatusCode == http.StatusUnauthorized
	case ErrForbidden:
		return e.StatusCode == http.StatusForbidden
	case ErrNotFound:
		return e.StatusCode == http.StatusNotFound
	case ErrRateLimited:
		return e.StatusCode == http.StatusTooManyRequests
	case ErrServer:
		return e.StatusCode >= 500
	}
	return false
}
