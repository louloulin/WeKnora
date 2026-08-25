package handler

import (
	"context"
	stderrors "errors"
	"net/http"

	applerrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// WikiAclServiceSurface is the slice of the ACL service the handler
// depends on. Defining it here (rather than importing the service
// package) keeps the handler trivially stubbable from tests.
type WikiAclServiceSurface interface {
	GetAcl(ctx context.Context, kbID string, slug string) (*types.WikiPageAcl, error)
	PutAcl(ctx context.Context, kbID string, slug string,
		req types.WikiPageAclSaveRequest, callerUserID string, callerRole string) (*types.WikiPageAcl, error)
}

// WikiAclHandler exposes the per-page access control REST surface added in
// Build #7. It reuses the wiki page group's KB validation so the route
// mounts under `wiki/pages/:slug/acl` without duplicating KB-existence
// checks.
type WikiAclHandler struct {
	aclService WikiAclServiceSurface
}

// NewWikiAclHandler wires the handler with its service dependency. The
// concrete *service.WikiAclService satisfies WikiAclServiceSurface.
func NewWikiAclHandler(aclService WikiAclServiceSurface) *WikiAclHandler {
	return &WikiAclHandler{aclService: aclService}
}

// GetAcl godoc
// @Summary      Get page-level ACL
// @Description  Returns the current ACL record for the wiki page.
// @Tags         Wiki
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        slug   path  string  true  "Page slug"
// @Success      200  {object}  types.WikiPageAcl
// @Failure      404  {object}  applerrors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages/{slug}/acl [get]
func (h *WikiAclHandler) GetAcl(c *gin.Context) {
	kbID, _, err := validateWikiKBForAcl(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	slug := getSlugParam(c)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page slug is required"})
		return
	}

	acl, err := h.aclService.GetAcl(c.Request.Context(), kbID, slug)
	if err != nil {
		logger.ErrorWithFields(c.Request.Context(), err, nil)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, acl)
}

// PutAcl godoc
// @Summary      Update page-level ACL
// @Description  Replace the ACL for a wiki page. Body must carry
// @Description  `base_revision` matching the current stored revision;
// @Description  mismatches return 409 so the client can reload and retry.
// @Tags         Wiki
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        slug   path  string  true  "Page slug"
// @Param        body   body  types.WikiPageAclSaveRequest  true  "ACL update"
// @Success      200  {object}  types.WikiPageAcl
// @Failure      400  {object}  applerrors.AppError
// @Failure      409  {object}  types.WikiPageAcl
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages/{slug}/acl [put]
func (h *WikiAclHandler) PutAcl(c *gin.Context) {
	kbID, _, err := validateWikiKBForAcl(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	slug := getSlugParam(c)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page slug is required"})
		return
	}

	var req types.WikiPageAclSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !types.IsValidWikiPageAclMode(req.Mode) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid acl mode"})
		return
	}

	userID, _ := types.UserIDFromContext(c.Request.Context())
	// Role is intentionally left at "user" — the underlying service does
	// not yet differentiate by role; audit log records the literal tag.
	// KB-level admin checks happen at the resolve path, not the write
	// path, so this stays simple until we need stricter write authorization.
	role := "user"

	updated, err := h.aclService.PutAcl(c.Request.Context(), kbID, slug, req, userID, role)
	if err != nil {
		if stderrors.Is(err, types.ErrWikiPageAclRevisionConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "acl revision conflict"})
			return
		}
		logger.ErrorWithFields(c.Request.Context(), err, nil)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// validateWikiKBForAcl pulls the kb_id path param and tenant id out of the
// context. Full KB existence is verified by the service layer's owner
// lookup so we don't pay for it on every ACL read.
func validateWikiKBForAcl(c *gin.Context) (string, uint64, error) {
	kbID := secutils.SanitizeForLog(c.Param("kb_id"))
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if kbID == "" {
		return "", 0, applerrors.NewBadRequestError("Knowledge base ID is required")
	}
	return kbID, tenantID, nil
}