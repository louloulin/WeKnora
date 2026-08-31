package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/mfasp"
	"github.com/Tencent/WeKnora/internal/types"
)

// MFAHandler exposes the per-user MFA lifecycle endpoints. The
// surface is intentionally small — 4 routes — because the real
// caller of Verify is the login flow (not exposed yet; this
// commit ships the verification primitives so the login work can
// land in the next change).
type MFAHandler struct {
	svc *service.MFAService
}

// NewMFAHandler constructs the handler.
func NewMFAHandler(svc *service.MFAService) *MFAHandler {
	return &MFAHandler{svc: svc}
}

// mfaUser pulls the authenticated user id off the context. We
// rely on the JWT auth middleware to set it.
func mfaUser(c *gin.Context) (string, bool) {
	v, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user_id missing in auth context"})
		return "", false
	}
	id, ok := v.(string)
	if !ok || id == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user_id"})
		return "", false
	}
	return id, true
}

// Enroll — POST /api/v1/mfa/enroll
func (h *MFAHandler) Enroll(c *gin.Context) {
	userID, ok := mfaUser(c)
	if !ok {
		return
	}
	var req types.MFAEnrollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.svc.Enroll(c.Request.Context(), userID, req.Name)
	if err != nil {
		respondMFAError(c, err)
		return
	}
	c.JSON(http.StatusCreated, &types.MFAEnrollResponse{
		ID:              res.Credential.ID,
		Name:            res.Credential.Name,
		Type:            res.Credential.Type,
		ProvisioningURI: res.ProvisioningURI,
		RecoveryCodes:   res.RecoveryCodes,
	})
}

// Verify — POST /api/v1/mfa/verify
func (h *MFAHandler) Verify(c *gin.Context) {
	userID, ok := mfaUser(c)
	if !ok {
		return
	}
	var req types.MFAVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.CredentialID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential_id is required"})
		return
	}
	if req.Code == "" && req.RecoveryCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code or recovery_code is required"})
		return
	}
	if err := h.svc.Verify(c.Request.Context(), req.CredentialID, req.Code, req.RecoveryCode); err != nil {
		respondMFAError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "user_id": userID})
}

// List — GET /api/v1/mfa
func (h *MFAHandler) List(c *gin.Context) {
	userID, ok := mfaUser(c)
	if !ok {
		return
	}
	rows, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		respondMFAError(c, err)
		return
	}
	// Strip sensitive fields before returning.
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{
			"id":           r.ID,
			"type":         r.Type,
			"name":         r.Name,
			"enabled":      r.Enabled,
			"enrolled_at":  r.EnrolledAt,
			"last_used_at": r.LastUsedAt,
		})
	}
	c.JSON(http.StatusOK, out)
}

// Disable — DELETE /api/v1/mfa/:id
func (h *MFAHandler) Disable(c *gin.Context) {
	userID, ok := mfaUser(c)
	if !ok {
		return
	}
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	// Ownership check: the credential must belong to the
	// authenticated user. We list rather than trust the URL.
	rows, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		respondMFAError(c, err)
		return
	}
	owned := false
	for _, r := range rows {
		if r.ID == id {
			owned = true
			break
		}
	}
	if !owned {
		respondMFAError(c, service.ErrMFACredentialNotFound)
		return
	}
	if err := h.svc.Disable(c.Request.Context(), id); err != nil {
		respondMFAError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// respondMFAError maps service sentinels to HTTP statuses. Generic
// errors degrade to 500 with a sanitized message.
func respondMFAError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrMFACredentialNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "mfa credential not found"})
	case errors.Is(err, service.ErrMFAAlreadyEnrolled):
		c.JSON(http.StatusConflict, gin.H{"error": "user already has an active mfa enrolment"})
	case errors.Is(err, service.ErrMFACodeInvalid):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "code invalid"})
	case errors.Is(err, mfasp.ErrInvalidRecovery), errors.Is(err, service.ErrMFARecoveryInvalid):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "recovery code invalid"})
	case errors.Is(err, service.ErrMFACredentialDisabled):
		c.JSON(http.StatusForbidden, gin.H{"error": "mfa credential disabled"})
	default:
		logger.Errorf(c.Request.Context(), "mfa handler error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
