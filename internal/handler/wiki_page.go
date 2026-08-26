package handler

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"encoding/csv"
	"encoding/json"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// WikiPageHandler handles HTTP requests for wiki page operations
type WikiPageHandler struct {
	wikiService      interfaces.WikiPageService
	kbService        interfaces.KnowledgeBaseService
	lintService      *service.WikiLintService
	auditService     interfaces.AuditLogService
	memoryService    interfaces.MemoryService
	batchJobService  interfaces.WikiBatchJobService
	// batchAuditRepo exposes the wiki batch audit log (Build #14).
	// May be nil for very old harness tests; the audit handlers return
	// 503 in that case so callers see a clear "not configured" error
	// rather than a misleading 500.
	batchAuditRepo interfaces.WikiBatchAuditRepository
	// batchFailureRepo exposes the per-slug failure ledger (Build #15).
	// Same nil-safe semantics as batchAuditRepo above.
	batchFailureRepo interfaces.WikiBatchFailureRepository
}

// NewWikiPageHandler creates a new wiki page handler.
//
// batchJobService may be nil — older builds (pre-#13) construct the
// handler without async batch support. The Batch* handlers fall back
// to synchronous execution in that case (Batch*Route returns sync
// when batchSvc is unset). Build #13.
//
// batchAuditRepo is Build #14 — pass nil for builds without the
// wiki_batch_job_audit table.
//
// batchFailureRepo is Build #15 — pass nil for builds without the
// wiki_batch_job_failures table; same 503 semantics as batchAuditRepo.
//
// Build #14 + Build #15.
func NewWikiPageHandler(
	wikiService interfaces.WikiPageService,
	kbService interfaces.KnowledgeBaseService,
	lintService *service.WikiLintService,
	auditService interfaces.AuditLogService,
	memoryService interfaces.MemoryService,
	batchJobService interfaces.WikiBatchJobService,
	batchAuditRepo interfaces.WikiBatchAuditRepository,
	batchFailureRepo interfaces.WikiBatchFailureRepository,
) *WikiPageHandler {
	return &WikiPageHandler{
		wikiService:      wikiService,
		kbService:        kbService,
		lintService:      lintService,
		auditService:     auditService,
		memoryService:    memoryService,
		batchJobService:  batchJobService,
		batchAuditRepo:   batchAuditRepo,
		batchFailureRepo: batchFailureRepo,
	}
}

// validateWikiKB validates that the KB exists and is a wiki type
func (h *WikiPageHandler) validateWikiKB(c *gin.Context) (string, uint64, error) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("kb_id"))
	tenantID := c.GetUint64(types.TenantIDContextKey.String())

	if kbID == "" {
		return "", 0, errors.NewBadRequestError("Knowledge base ID is required")
	}

	kb, err := h.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return "", 0, errors.NewNotFoundError("Knowledge base not found")
	}

	if !kb.IsWikiEnabled() {
		return "", 0, errors.NewBadRequestError("Wiki feature is not enabled for this knowledge base")
	}

	return kbID, tenantID, nil
}

// getSlugParam extracts and cleans the slug from gin's wildcard path param
func getSlugParam(c *gin.Context) string {
	slug := c.Param("slug")
	// gin wildcard params include a leading "/"
	slug = strings.TrimPrefix(slug, "/")
	return strings.TrimSpace(slug)
}

// ListPages godoc
// @Summary      List wiki pages
// @Description  List wiki pages with optional filtering and pagination
// @Tags         Wiki
// @Produce      json
// @Param        kb_id      path      string  true   "Knowledge base ID"
// @Param        page_type  query     string  false  "Filter by page type; comma-separated for multiple (e.g. entity,concept)"
// @Param        status     query     string  false  "Filter by status"
// @Param        query      query     string  false  "Full-text search"
// @Param        page       query     int     false  "Page number"
// @Param        page_size  query     int     false  "Page size"
// @Param        sort_by    query     string  false  "Sort field"
// @Param        sort_order query     string  false  "Sort order (asc/desc)"
// @Success      200  {object}  types.WikiPageListResponse
// @Failure      400  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages [get]
func (h *WikiPageHandler) ListPages(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	categoryPath := parseWikiCategoryPath(c.Query("category_path"))
	// folder_id is an exact placement filter. An explicitly-present but empty
	// value means "root" (folder_id = ''); an absent param means "no filter".
	var folderID *string
	if raw, ok := c.GetQuery("folder_id"); ok {
		raw = strings.TrimSpace(raw)
		folderID = &raw
	}
	var categoryDepth *int
	if raw := c.Query("category_depth"); raw != "" {
		if depth, parseErr := strconv.Atoi(raw); parseErr == nil && depth >= 0 {
			categoryDepth = &depth
		}
	}

	req := &types.WikiPageListRequest{
		KnowledgeBaseID: kbID,
		PageType:        c.Query("page_type"),
		Status:          c.Query("status"),
		Query:           c.Query("query"),
		FolderID:        folderID,
		CategoryPath:    types.StringArray(categoryPath),
		CategoryDepth:   categoryDepth,
		Page:            page,
		PageSize:        pageSize,
		SortBy:          c.DefaultQuery("sort_by", "updated_at"),
		SortOrder:       c.DefaultQuery("sort_order", "desc"),
	}

	resp, err := h.wikiService.ListPages(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ListFolders godoc
// @Summary      List wiki folders
// @Description  Retrieve the direct child folders of a parent folder (parent_id empty = root level), each with its page count and a has-children flag for the directory tree.
// @Tags         Wiki
// @Produce      json
// @Param        kb_id     path   string  true   "Knowledge base ID"
// @Param        parent_id query  string  false  "Parent folder id (empty = root)"
// @Success      200  {object}  types.WikiFolderListResponse
// @Failure      400  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/folders [get]
func (h *WikiPageHandler) ListFolders(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	parentID := strings.TrimSpace(c.Query("parent_id"))
	var pageTypes []string
	if raw := strings.TrimSpace(c.Query("page_types")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if p := strings.TrimSpace(part); p != "" {
				pageTypes = append(pageTypes, p)
			}
		}
	}
	folders, err := h.wikiService.ListChildFolders(c.Request.Context(), kbID, parentID, pageTypes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if folders == nil {
		folders = []types.WikiFolderNode{}
	}
	c.JSON(http.StatusOK, types.WikiFolderListResponse{ParentID: parentID, Folders: folders})
}

// CreateFolder godoc
// @Summary      Create a wiki folder
// @Description  Create a new (initially empty) directory node under parent_id
// @Tags         Wiki
// @Accept       json
// @Produce      json
// @Param        kb_id  path  string                       true  "Knowledge base ID"
// @Param        folder body  types.WikiFolderCreateRequest true  "Folder data"
// @Success      201  {object}  types.WikiFolder
// @Failure      400  {object}  errors.AppError
// @Failure      409  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/folders [post]
func (h *WikiPageHandler) CreateFolder(c *gin.Context) {
	kbID, tenantID, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req types.WikiFolderCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	folder, err := h.wikiService.CreateFolder(c.Request.Context(), kbID, tenantID, strings.TrimSpace(req.ParentID), req.Name)
	if err != nil {
		writeWikiFolderError(c, err)
		return
	}
	c.JSON(http.StatusCreated, folder)
}

// UpdateFolder godoc
// @Summary      Rename or move a wiki folder
// @Description  Rename and/or reparent a folder; the whole subtree's paths and the affected pages' cached paths are recomputed
// @Tags         Wiki
// @Accept       json
// @Produce      json
// @Param        kb_id     path  string                        true  "Knowledge base ID"
// @Param        folder_id path  string                        true  "Folder ID"
// @Param        folder    body  types.WikiFolderUpdateRequest true  "Folder update"
// @Success      200  {object}  types.WikiFolder
// @Failure      400  {object}  errors.AppError
// @Failure      404  {object}  errors.AppError
// @Failure      409  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/folders/{folder_id} [put]
func (h *WikiPageHandler) UpdateFolder(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	folderID := secutils.SanitizeForLog(c.Param("folder_id"))
	if folderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Folder ID is required"})
		return
	}
	var req types.WikiFolderUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	folder, err := h.wikiService.RenameOrMoveFolder(
		c.Request.Context(), kbID, folderID, req.Name, strings.TrimSpace(req.ParentID), req.MoveParent)
	if err != nil {
		writeWikiFolderError(c, err)
		return
	}
	c.JSON(http.StatusOK, folder)
}

// DeleteFolder godoc
// @Summary      Delete an empty wiki folder
// @Description  Delete a folder that has no pages and no child folders
// @Tags         Wiki
// @Param        kb_id     path  string  true  "Knowledge base ID"
// @Param        folder_id path  string  true  "Folder ID"
// @Success      204
// @Failure      400  {object}  errors.AppError
// @Failure      404  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/folders/{folder_id} [delete]
func (h *WikiPageHandler) DeleteFolder(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	folderID := secutils.SanitizeForLog(c.Param("folder_id"))
	if folderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Folder ID is required"})
		return
	}
	if err := h.wikiService.DeleteFolder(c.Request.Context(), kbID, folderID); err != nil {
		writeWikiFolderError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// MovePage godoc
// @Summary      Move a wiki page into a folder
// @Description  Relocate a page (identified by slug in the body) into a folder (folder_id empty = root); the page's cached category path is recomputed
// @Tags         Wiki
// @Accept       json
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        move   body  types.WikiPageMoveRequest true "Move target"
// @Success      200  {object}  types.WikiPage
// @Failure      404  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/move-page [put]
func (h *WikiPageHandler) MovePage(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req types.WikiPageMoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page slug is required"})
		return
	}
	page, err := h.wikiService.MovePage(c.Request.Context(), kbID, slug, strings.TrimSpace(req.FolderID))
	if err != nil {
		writeWikiFolderError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

// BatchMovePages godoc
// @Summary      Batch-move wiki pages into a folder
// @Description  Relocate up to MaxWikiBatchSize pages into the same folder. Requests below WikiBatchAsyncThreshold run synchronously and return the WikiBatchResult directly; larger requests enqueue an async job and return { kind: "job", job } with HTTP 202. Slugs are deduped server-side.
// @Tags         Wiki
// @Accept       json
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        body   body  types.WikiPageBatchMoveRequest true "Batch move payload"
// @Success      200  {object}  types.WikiBatchRouteResult
// @Success      202  {object}  types.WikiBatchRouteResult
// @Failure      400  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages/batch-move [post]
func (h *WikiPageHandler) BatchMovePages(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req types.WikiPageBatchMoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	if err := validateBatchSlugs(req.Slugs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID, _ := types.UserIDFromContext(c.Request.Context())
	result, err := h.wikiService.BatchMovePagesRoute(c.Request.Context(), kbID, req.Slugs, strings.TrimSpace(req.FolderID), userID)
	if err != nil {
		if respondBatchServiceError(c, err) {
			return
		}
		writeWikiFolderError(c, err)
		return
	}
	h.writeBatchRouteResult(c, result)
}

// BatchDeletePages godoc
// @Summary      Batch soft-delete wiki pages
// @Description  Soft-delete up to MaxWikiBatchSize pages. Each successful row cascades removeInLinks + chunk deletion exactly like DeletePage. Auto-routes to async when >= WikiBatchAsyncThreshold slugs.
// @Tags         Wiki
// @Accept       json
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        body   body  types.WikiPageBatchDeleteRequest true "Batch delete payload"
// @Success      200  {object}  types.WikiBatchRouteResult
// @Success      202  {object}  types.WikiBatchRouteResult
// @Failure      400  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages/batch-delete [post]
func (h *WikiPageHandler) BatchDeletePages(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req types.WikiPageBatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	if err := validateBatchSlugs(req.Slugs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID, _ := types.UserIDFromContext(c.Request.Context())
	result, err := h.wikiService.BatchDeletePagesRoute(c.Request.Context(), kbID, req.Slugs, userID)
	if err != nil {
		if respondBatchServiceError(c, err) {
			return
		}
		writeWikiFolderError(c, err)
		return
	}
	h.writeBatchRouteResult(c, result)
}

// BatchUpdatePageStatus godoc
// @Summary      Batch update wiki page status
// @Description  Rewrite `status` (draft / published / archived) for up to MaxWikiBatchSize pages. Bookkeeping-only — does not bump `version`. Auto-routes to async when >= WikiBatchAsyncThreshold slugs; status jobs are NOT undoable.
// @Tags         Wiki
// @Accept       json
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        body   body  types.WikiPageBatchStatusRequest true "Batch status payload"
// @Success      200  {object}  types.WikiBatchRouteResult
// @Success      202  {object}  types.WikiBatchRouteResult
// @Failure      400  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages/batch-status [post]
func (h *WikiPageHandler) BatchUpdatePageStatus(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req types.WikiPageBatchStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	if err := validateBatchSlugs(req.Slugs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID, _ := types.UserIDFromContext(c.Request.Context())
	result, err := h.wikiService.BatchUpdatePageStatusRoute(c.Request.Context(), kbID, req.Slugs, req.Status, userID)
	if err != nil {
		if respondBatchServiceError(c, err) {
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.writeBatchRouteResult(c, result)
}

// writeBatchRouteResult centralises the HTTP-status decision for the
// three Batch*Route responses: 200 for sync, 202 for queued async. The
// response body shape is the same (WikiBatchRouteResult) — only the
// status code differs.
//
// Build #13.
func (h *WikiPageHandler) writeBatchRouteResult(c *gin.Context, r *types.WikiBatchRouteResult) {
	if r == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "nil batch route result"})
		return
	}
	status := http.StatusOK
	if r.Kind == "job" {
		status = http.StatusAccepted
	}
	c.JSON(status, r)
}

// GetBatchJob godoc
// @Summary      Get an async batch job's status
// @Description  Returns the WikiBatchJob row plus the worker pool's result blob. Use to poll progress after a 202 response from one of the batch-* endpoints.
// @Tags         Wiki
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        job_id path  string  true  "Batch job id"
// @Success      200  {object}  types.WikiBatchJob
// @Failure      404  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/batch-jobs/{job_id} [get]
func (h *WikiPageHandler) GetBatchJob(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	jobID := strings.TrimSpace(c.Param("job_id"))
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job_id is required"})
		return
	}
	job, err := h.batchJobService.GetJob(c.Request.Context(), jobID)
	if err != nil {
		h.respondBatchJobError(c, kbID, err)
		return
	}
	if job.KnowledgeBaseID != kbID {
		// Same as "not found" to avoid leaking cross-KB existence.
		c.JSON(http.StatusNotFound, gin.H{"error": "batch job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}

// UndoBatchJob godoc
// @Summary      Roll back a finished batch job
// @Description  Reverses a `move` or `delete` job. Returns the updated WikiBatchJob with `expires_at` cleared. 422 for status / tag (not undoable), 410 when the 7-day window has passed.
// @Tags         Wiki
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        job_id path  string  true  "Batch job id"
// @Success      200  {object}  types.WikiBatchJob
// @Failure      404  {object}  errors.AppError
// @Failure      410  {object}  errors.AppError
// @Failure      422  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/batch-jobs/{job_id}/undo [post]
func (h *WikiPageHandler) UndoBatchJob(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	jobID := strings.TrimSpace(c.Param("job_id"))
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job_id is required"})
		return
	}
	userID, _ := types.UserIDFromContext(c.Request.Context())
	job, err := h.batchJobService.UndoJob(c.Request.Context(), kbID, jobID, userID)
	if err != nil {
		h.respondBatchJobError(c, kbID, err)
		return
	}
	c.JSON(http.StatusOK, job)
}

// CancelBatchJob godoc
// @Summary      Cancel a queued batch job
// @Description  Aborts a queued batch job before any worker picks it up. Returns the cleared WikiBatchJob. 409 once the job has started running.
// @Tags         Wiki
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        job_id path  string  true  "Batch job id"
// @Success      200  {object}  types.WikiBatchJob
// @Failure      404  {object}  errors.AppError
// @Failure      409  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/batch-jobs/{job_id}/cancel [post]
//
// Build #14.
func (h *WikiPageHandler) CancelBatchJob(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	jobID := strings.TrimSpace(c.Param("job_id"))
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job_id is required"})
		return
	}
	userID, _ := types.UserIDFromContext(c.Request.Context())
	job, err := h.batchJobService.CancelJob(c.Request.Context(), kbID, jobID, userID)
	if err != nil {
		h.respondBatchJobError(c, kbID, err)
		return
	}
	c.JSON(http.StatusOK, job)
}

// GetBatchJobAudit godoc
// @Summary      Audit log for one batch job
// @Description  Returns every audit event recorded against a single batch job, oldest-first. Per-job cardinality is bounded (<= 7 events) so no pagination.
// @Tags         Wiki
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        job_id path  string  true  "Batch job id"
// @Success      200  {array}   types.WikiBatchJobAuditEvent
// @Failure      400  {object}  errors.AppError
// @Failure      503  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/batch-jobs/{job_id}/audit [get]
//
// Build #14.
func (h *WikiPageHandler) GetBatchJobAudit(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if h.batchAuditRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "batch audit log is not configured"})
		return
	}
	jobID := strings.TrimSpace(c.Param("job_id"))
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job_id is required"})
		return
	}
	events, err := h.batchAuditRepo.ListByJobID(c.Request.Context(), kbID, jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, events)
}

// GetBatchJobFailures godoc
// @Summary      Per-slug failures for one batch job
// @Description  Returns the failures table rows for a single batch job (Build #15), oldest-first, paginated. Each row identifies one slug whose per-slug execution failed and the classifier-friendly error code. Optional `code` filter narrows to one bucket; the `groups` slice is always computed over the full filtered set so the drawer's code tabs stay accurate even when the user is deep in pagination.
// @Tags         Wiki
// @Produce      json
// @Param        kb_id     path  string  true  "Knowledge base ID"
// @Param        job_id    path  string  true  "Batch job id"
// @Param        code      query string  false  "Filter by error code (not_found, folder_not_found, folder_conflict, folder_not_empty, kb_mismatch, internal)"
// @Param        page      query int     false  "1-based page number (default 1)"
// @Param        page_size query int     false  "Page size, 1-200 (default 50)"
// @Success      200  {object}  types.WikiBatchFailureListResponse
// @Failure      400  {object}  errors.AppError
// @Failure      503  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/batch-jobs/{job_id}/failures [get]
//
// Build #15.
func (h *WikiPageHandler) GetBatchJobFailures(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if h.batchFailureRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "batch failure log is not configured"})
		return
	}
	jobID := strings.TrimSpace(c.Param("job_id"))
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job_id is required"})
		return
	}
	page, pageSize := parsePagination(c, 50, 200)
	code := strings.TrimSpace(c.Query("code"))
	failures, groups, total, err := h.batchFailureRepo.ListByJobID(
		c.Request.Context(), kbID, jobID, code, page, pageSize,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, types.WikiBatchFailureListResponse{
		Failures:  failures,
		Groups:    groups,
		Total:     int(total),
		Page:      page,
		PageSize:  pageSize,
	})
}

// ListBatchJobAudit godoc
// @Summary      KB-wide audit log (paginated)
// @Description  Lists audit events for a knowledge base, newest-first. Filterable by actor / action / since. `since` is bounded to the last 90 days per D4.
// @Tags         Wiki
// @Produce      json
// @Param        kb_id     path  string  true   "Knowledge base ID"
// @Param        actor     query string  false  "Filter by actor id"
// @Param        action    query string  false  "Filter by action (enqueue, start, finish, undo_request, undo_done, cancel, expire)"
// @Param        since     query string  false  "Lower bound (RFC3339). Capped at 90 days before now."
// @Param        page      query int     false  "1-based page number (default 1)"
// @Param        page_size query int     false  "Page size, 1-200 (default 50)"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  errors.AppError
// @Failure      503  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/batch-audit [get]
//
// Build #14.
func (h *WikiPageHandler) ListBatchJobAudit(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if h.batchAuditRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "batch audit log is not configured"})
		return
	}
	page, pageSize := parsePagination(c, 50, 200)
	actor := strings.TrimSpace(c.Query("actor"))
	action := types.WikiBatchAuditAction(strings.TrimSpace(c.Query("action")))
	if action != "" && !types.IsValidWikiBatchAuditAction(action) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action filter"})
		return
	}
	since, sinceErr := parseAuditSince(c, 90*24*time.Hour)
	if sinceErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sinceErr.Error()})
		return
	}
	events, total, err := h.batchAuditRepo.ListByKB(c.Request.Context(), kbID, actor, action, since, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"events":    events,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ExportBatchJobAuditCsv godoc
// @Summary      Export KB batch audit log as CSV
// @Description  Streams the same events as ListBatchJobAudit but as RFC 4180 CSV. `since` lower bound capped at 90 days; no upper bound.
// @Tags         Wiki
// @Produce      text/csv
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        actor  query string false "Filter by actor id"
// @Param        since  query string false "Lower bound (RFC3339). Capped at 90 days before now."
// @Success      200  {file}  string
// @Failure      400  {object}  errors.AppError
// @Failure      503  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/batch-audit/export [get]
//
// Build #14.
func (h *WikiPageHandler) ExportBatchJobAuditCsv(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if h.batchAuditRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "batch audit log is not configured"})
		return
	}
	actor := strings.TrimSpace(c.Query("actor"))
	since, sinceErr := parseAuditSince(c, 90*24*time.Hour)
	if sinceErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sinceErr.Error()})
		return
	}
	events, _, err := h.batchAuditRepo.ListByKB(c.Request.Context(), kbID, actor, "", since, 1, 10000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="wiki-batch-audit-`+kbID+`.csv"`)
	w := csv.NewWriter(c.Writer)
	defer w.Flush()
	_ = w.Write([]string{"id", "occurred_at", "action", "actor_id", "batch_job_id", "metadata"})
	for _, e := range events {
		meta, _ := json.Marshal(e.Metadata)
		_ = w.Write([]string{
			strconv.FormatInt(e.ID, 10),
			e.OccurredAt.UTC().Format(time.RFC3339Nano),
			string(e.Action),
			e.ActorID,
			e.BatchJobID,
			string(meta),
		})
	}
}

// parseAuditSince parses the `since` query string and rejects values
// older than `maxAge`. Zero time → no lower bound. Returns the parsed
// time or an error suitable for a 400 response.
//
// Build #14.
func parseAuditSince(c *gin.Context, maxAge time.Duration) (time.Time, error) {
	raw := strings.TrimSpace(c.Query("since"))
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid since: must be RFC3339")
	}
	if maxAge > 0 {
		cutoff := time.Now().Add(-maxAge)
		if t.Before(cutoff) {
			return time.Time{}, fmt.Errorf("since exceeds the %s retention window", maxAge)
		}
	}
	return t, nil
}

// parsePagination extracts page / page_size from the request, applying
// sensible defaults and clamping page_size to the supplied cap.
//
// Build #14.
func parsePagination(c *gin.Context, defaultSize, maxSize int) (page, pageSize int) {
	page = 1
	pageSize = defaultSize
	if v, err := strconv.Atoi(strings.TrimSpace(c.Query("page"))); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(c.Query("page_size"))); err == nil && v > 0 {
		pageSize = v
	}
	if pageSize > maxSize {
		pageSize = maxSize
	}
	return page, pageSize
}

// respondBatchJobError maps WikiBatchJob sentinel errors to HTTP statuses.
// Cross-KB access is folded into 404 (not 403) so existence doesn't
// leak across knowledge bases.
//
// Build #13. Build #14 adds ErrWikiBatchJobNotCancellable → 409.
func (h *WikiPageHandler) respondBatchJobError(c *gin.Context, kbID string, err error) {
	switch {
	case stderrors.Is(err, types.ErrWikiBatchJobNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "batch job not found"})
	case stderrors.Is(err, types.ErrWikiBatchJobNotUndoable):
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "batch job type is not undoable",
			"code":  "not_undoable",
		})
	case stderrors.Is(err, types.ErrWikiBatchJobExpired):
		c.JSON(http.StatusGone, gin.H{"error": "batch job undo window expired"})
	case stderrors.Is(err, types.ErrWikiBatchJobNotCancellable):
		c.JSON(http.StatusConflict, gin.H{
			"error": "batch job is no longer cancellable",
			"code":  "not_cancellable",
		})
	default:
		writeWikiFolderError(c, err)
	}
}

// respondBatchServiceError maps request-level batch errors (currently only
// cross-KB collisions) to the 400 contract. Returns true when the error
// was recognised and a response written, so callers can short-circuit.
// Any unrecognised error falls through to the existing per-handler
// fallback (writeWikiFolderError / generic 500 / etc.).
//
// Build #12.
func respondBatchServiceError(c *gin.Context, err error) bool {
	if !types.IsWikiBatchKBMismatch(err) {
		return false
	}
	var mismatch *types.WikiBatchKBMismatchError
	if stderrors.As(err, &mismatch) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      "kb_mismatch",
			"slug":      mismatch.Slug,
			"actual_kb": mismatch.ActualKB,
			"error":     err.Error(),
		})
		return true
	}
	c.JSON(http.StatusBadRequest, gin.H{
		"code":  "kb_mismatch",
		"error": err.Error(),
	})
	return true
}

// validateBatchSlugs enforces MaxWikiBatchSize and rejects empty payloads
// with the same 400 contract across all three batch endpoints (D4, A5, A9).
func validateBatchSlugs(slugs []string) error {
	if len(slugs) == 0 {
		return errors.New("slugs must not be empty")
	}
	if len(slugs) > types.MaxWikiBatchSize {
		return fmt.Errorf("too many slugs: %d (max %d)", len(slugs), types.MaxWikiBatchSize)
	}
	return nil
}
	case stderrors.Is(err, repository.ErrWikiFolderNotFound), stderrors.Is(err, repository.ErrWikiPageNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case stderrors.Is(err, repository.ErrWikiFolderConflict), stderrors.Is(err, repository.ErrWikiFolderNotEmpty):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func parseWikiCategoryPath(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// CreatePage godoc
// @Summary      Create a wiki page
// @Description  Create a new wiki page in the knowledge base
// @Tags         Wiki
// @Accept       json
// @Produce      json
// @Param        kb_id  path  string          true  "Knowledge base ID"
// @Param        page   body  types.WikiPage  true  "Wiki page data"
// @Success      201  {object}  types.WikiPage
// @Failure      400  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages [post]
func (h *WikiPageHandler) CreatePage(c *gin.Context) {
	kbID, tenantID, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var page types.WikiPage
	if err := c.ShouldBindJSON(&page); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	page.KnowledgeBaseID = kbID
	page.TenantID = tenantID
	page.PageType = strings.TrimSpace(page.PageType)
	page.Status = strings.TrimSpace(page.Status)
	if page.PageType != "" && !types.IsValidWikiPageType(page.PageType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page_type: " + page.PageType})
		return
	}
	if page.Status != "" && !types.IsValidWikiPageStatus(page.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status: " + page.Status})
		return
	}

	ctx := types.WithWikiEditSource(c.Request.Context(), types.WikiEditSourceUser)
	created, err := h.wikiService.CreatePage(ctx, &page)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.recordManualWikiActivity(ctx, created, "manual_create")
	c.JSON(http.StatusCreated, created)
}

// recordManualWikiActivity projects a manual page mutation directly into the
// knowledge-base activity feed. Activity recording is best-effort and must not
// fail the edit itself.
func (h *WikiPageHandler) recordManualWikiActivity(
	ctx context.Context, page *types.WikiPage, action string,
) {
	if page == nil {
		return
	}
	service.RecordWikiContentActivity(ctx, h.auditService, page.TenantID,
		page.KnowledgeBaseID, map[string]int{action: 1})
}

// GetPage godoc
// @Summary      Get a wiki page by slug
// @Description  Retrieve a wiki page by its slug
// @Tags         Wiki
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        slug   path  string  true  "Page slug"
// @Success      200  {object}  types.WikiPage
// @Failure      404  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages/{slug} [get]
func (h *WikiPageHandler) GetPage(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	slug := getSlugParam(c)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page slug is required"})
		return
	}

	page, err := h.wikiService.GetPageBySlug(c.Request.Context(), kbID, slug)
	if err != nil {
		if stderrors.Is(err, repository.ErrWikiPageNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Wiki page not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, page)
}

// GetPageBacklinks godoc
// @Summary      List pages that link to this wiki page
// @Description  Returns the resolved set of pages within the knowledge base
// @Description  whose body contains a [[<slug>]] reference to this page.
// @Description  Result is ordered by updated_at desc with slug alphabetical
// @Description  as the tiebreaker. Orphan slugs (in_links entries whose
// @Description  target page has been deleted) are filtered server-side.
// @Description  An empty array + 200 is returned when the page exists
// @Description  but has no inbound links; 404 when the page itself does
// @Description  not exist. Build #11.
// @Tags         Wiki
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        slug   path  string  true  "Page slug"
// @Success      200  {array}  types.WikiPageBacklink
// @Failure      400  {object}  errors.AppError
// @Failure      404  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages/{slug}/backlinks [get]
func (h *WikiPageHandler) GetPageBacklinks(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	slug := getSlugParam(c)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page slug is required"})
		return
	}
	backlinks, err := h.wikiService.ListPageBacklinks(c.Request.Context(), kbID, slug)
	if err != nil {
		if stderrors.Is(err, repository.ErrWikiPageNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Wiki page not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if backlinks == nil {
		backlinks = []*types.WikiPageBacklink{}
	}
	c.JSON(http.StatusOK, backlinks)
}

// UpdatePage godoc
// @Summary      Update a wiki page
// @Description  Partially update a wiki page by slug. Absent fields keep
// @Description  their stored value. When `version` is > 0 it acts as an
// @Description  optimistic-lock guard: a mismatch with the stored version
// @Description  returns 409 together with the current version so the client
// @Description  can reload and re-apply.
// @Tags         Wiki
// @Accept       json
// @Produce      json
// @Param        kb_id  path  string                       true  "Knowledge base ID"
// @Param        slug   path  string                       true  "Page slug"
// @Param        page   body  types.WikiPageUpdateRequest  true  "Fields to update"
// @Success      200  {object}  types.WikiPage
// @Failure      400  {object}  errors.AppError
// @Failure      404  {object}  errors.AppError
// @Failure      409  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages/{slug} [put]
func (h *WikiPageHandler) UpdatePage(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	slug := getSlugParam(c)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page slug is required"})
		return
	}

	var req types.WikiPageUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	ctx := types.WithWikiEditSource(c.Request.Context(), types.WikiEditSourceUser)

	existing, err := h.wikiService.GetPageBySlug(ctx, kbID, slug)
	if err != nil {
		if stderrors.Is(err, repository.ErrWikiPageNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Wiki page not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if req.Version > 0 && req.Version != existing.Version {
		c.JSON(http.StatusConflict, gin.H{
			"error":           "Wiki page was modified by someone else",
			"current_version": existing.Version,
		})
		return
	}

	// Merge the provided fields onto the stored page so absent fields keep
	// their value — the service's UpdatePage semantics are "full intended
	// state", and sending it a half-empty struct would clear real data.
	page := *existing
	if req.Title != nil {
		page.Title = strings.TrimSpace(*req.Title)
	}
	if req.Content != nil {
		page.Content = *req.Content
	}
	if req.ContentHTML != nil {
		page.ContentHTML = *req.ContentHTML
	}
	if req.Summary != nil {
		page.Summary = *req.Summary
	}
	if req.PageType != nil {
		page.PageType = strings.TrimSpace(*req.PageType)
		if !types.IsValidWikiPageType(page.PageType) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page_type: " + page.PageType})
			return
		}
	}
	if req.Status != nil {
		page.Status = strings.TrimSpace(*req.Status)
		if !types.IsValidWikiPageStatus(page.Status) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status: " + page.Status})
			return
		}
	}
	if req.Aliases != nil {
		page.Aliases = *req.Aliases
	}

	updated, err := h.wikiService.UpdatePage(ctx, &page)
	if err != nil {
		switch {
		case stderrors.Is(err, repository.ErrWikiPageNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Wiki page not found"})
		case stderrors.Is(err, repository.ErrWikiPageConflict):
			c.JSON(http.StatusConflict, gin.H{"error": "Wiki page was modified by someone else"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	if updated.Version != existing.Version {
		h.recordManualWikiActivity(ctx, updated, "manual_edit")
	}
	c.JSON(http.StatusOK, updated)
}

// ListRevisions godoc
// @Summary      List wiki page revisions
// @Description  Returns the stored historical snapshots for a page, newest
// @Description  first (content omitted), plus the current version. Passing
// @Description  `version` returns that single snapshot with full content.
// @Tags         Wiki
// @Produce      json
// @Param        kb_id    path   string  true   "Knowledge base ID"
// @Param        slug     path   string  true   "Page slug"
// @Param        version  query  int     false  "Return this single revision with content"
// @Param        limit    query  int     false  "Page size (default 50, max 200)"
// @Param        offset   query  int     false  "Offset into the newest-first list"
// @Success      200  {object}  types.WikiPageRevisionListResponse
// @Failure      404  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/revisions/{slug} [get]
func (h *WikiPageHandler) ListRevisions(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	slug := getSlugParam(c)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page slug is required"})
		return
	}

	ctx := c.Request.Context()

	if raw := c.Query("version"); raw != "" {
		version, parseErr := strconv.Atoi(raw)
		if parseErr != nil || version < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version"})
			return
		}
		rev, err := h.wikiService.GetRevision(ctx, kbID, slug, version)
		if err != nil {
			if stderrors.Is(err, repository.ErrWikiPageNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Wiki page revision not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, rev)
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if offset < 0 {
		offset = 0
	}

	resp, err := h.wikiService.ListRevisions(ctx, kbID, slug, limit, offset)
	if err != nil {
		if stderrors.Is(err, repository.ErrWikiPageNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Wiki page not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// RevertPage godoc
// @Summary      Revert a wiki page to an earlier revision
// @Description  Rolls the page (slug in the body, like move-page) back to
// @Description  the content of the given stored revision. Applied as a
// @Description  regular edit: the pre-revert state is snapshotted and the
// @Description  version advances.
// @Tags         Wiki
// @Accept       json
// @Produce      json
// @Param        kb_id   path  string                       true  "Knowledge base ID"
// @Param        revert  body  types.WikiPageRevertRequest  true  "Revert target"
// @Success      200  {object}  types.WikiPage
// @Failure      400  {object}  errors.AppError
// @Failure      404  {object}  errors.AppError
// @Failure      409  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/revert [post]
func (h *WikiPageHandler) RevertPage(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var req types.WikiPageRevertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page slug is required"})
		return
	}
	if req.Version < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version"})
		return
	}

	ctx := c.Request.Context()
	updated, err := h.wikiService.RevertPageToVersion(ctx, kbID, slug, req.Version)
	if err != nil {
		switch {
		case stderrors.Is(err, repository.ErrWikiPageNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Wiki page or revision not found"})
		case stderrors.Is(err, repository.ErrWikiPageConflict):
			c.JSON(http.StatusConflict, gin.H{"error": "Wiki page was modified by someone else"})
		case stderrors.Is(err, service.ErrWikiRevertToCurrentVersion):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	h.recordManualWikiActivity(ctx, updated, "revert")
	c.JSON(http.StatusOK, updated)
}

// DeletePage godoc
// @Summary      Delete a wiki page
// @Description  Soft-delete a wiki page by slug
// @Tags         Wiki
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        slug   path  string  true  "Page slug"
// @Success      204
// @Failure      404  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages/{slug} [delete]
func (h *WikiPageHandler) DeletePage(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	slug := getSlugParam(c)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page slug is required"})
		return
	}

	ctx := c.Request.Context()
	// Load first so the log entry can carry the page title after deletion.
	page, _ := h.wikiService.GetPageBySlug(ctx, kbID, slug)

	if err := h.wikiService.DeletePage(ctx, kbID, slug); err != nil {
		if stderrors.Is(err, repository.ErrWikiPageNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Wiki page not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.recordManualWikiActivity(ctx, page, "manual_delete")
	c.Status(http.StatusNoContent)
}

// GetIndex godoc
// @Summary      Get wiki index view
// @Description  Returns the wiki index as intro text plus per-type paginated
// @Description  directory groups. The heavy directory markdown that used to
// @Description  live in wiki_pages.content was replaced with this structured
// @Description  response so a KB with tens of thousands of pages no longer
// @Description  materializes megabytes of TEXT on every index open.
// @Tags         Wiki
// @Produce      json
// @Param        kb_id   path   string  true   "Knowledge base ID"
// @Param        types   query  string  false  "Comma-separated page types (default: all content types)"
// @Param        limit   query  int     false  "Per-group window size, 1-200 (default 50)"
// @Param        cursor  query  string  false  "Opaque offset cursor from previous response"
// @Success      200  {object}  types.WikiIndexResponse
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/index [get]
func (h *WikiPageHandler) GetIndex(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var pageTypes []string
	if raw := c.Query("types"); raw != "" {
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				pageTypes = append(pageTypes, t)
			}
		}
	}

	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if v, convErr := strconv.Atoi(raw); convErr == nil && v > 0 {
			limit = v
		}
	}

	resp, err := h.wikiService.GetIndexView(c.Request.Context(), kbID, pageTypes, limit, c.Query("cursor"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Graph query parameter bounds. The defaults cap an `overview` request at
// 500 nodes — comfortably renderable in the frontend's hand-rolled SVG
// force simulation — while the hard max of 2000 is the upper bound a
// power user can opt into before rendering gets choppy. Ego depth is
// capped at 3 hops because the node population grows super-linearly with
// depth and wider searches are better served by repeated ego jumps.
const (
	wikiGraphDefaultLimit = 500
	wikiGraphMaxLimit     = 2000
	wikiGraphMaxDepth     = 3
	wikiGraphDefaultDepth = 1
)

// GetGraph godoc
// @Summary      Get wiki link graph
// @Description  Returns a slice of the wiki link graph for visualization. Supports
// @Description  `mode=overview` (top-N most-connected pages, default) and
// @Description  `mode=ego` (BFS neighborhood of a center slug) to keep response
// @Description  size tractable for knowledge bases with tens of thousands of pages.
// @Tags         Wiki
// @Produce      json
// @Param        kb_id   path  string  true   "Knowledge base ID"
// @Param        mode    query string  false  "overview (default) | ego"
// @Param        center  query string  false  "Center slug for ego mode"
// @Param        depth   query int     false  "Ego BFS depth (1-3, default 1)"
// @Param        types   query string  false  "Comma-separated page_type allow-list"
// @Param        limit   query int     false  "Max nodes to return (default 500, max 2000)"
// @Success      200  {object}  types.WikiGraphData
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/graph [get]
func (h *WikiPageHandler) GetGraph(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	mode := strings.TrimSpace(c.Query("mode"))
	if mode == "" {
		mode = types.WikiGraphModeOverview
	}
	if mode != types.WikiGraphModeOverview && mode != types.WikiGraphModeEgo {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be 'overview' or 'ego'"})
		return
	}

	center := strings.TrimSpace(c.Query("center"))
	if mode == types.WikiGraphModeEgo && center == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "center is required when mode=ego"})
		return
	}

	depth := wikiGraphDefaultDepth
	if v := c.Query("depth"); v != "" {
		parsed, parseErr := strconv.Atoi(v)
		if parseErr != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "depth must be a positive integer"})
			return
		}
		if parsed > wikiGraphMaxDepth {
			parsed = wikiGraphMaxDepth
		}
		depth = parsed
	}

	limit := wikiGraphDefaultLimit
	if v := c.Query("limit"); v != "" {
		parsed, parseErr := strconv.Atoi(v)
		if parseErr != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
			return
		}
		if parsed > wikiGraphMaxLimit {
			parsed = wikiGraphMaxLimit
		}
		limit = parsed
	}

	var typesFilter []string
	if v := strings.TrimSpace(c.Query("types")); v != "" {
		for _, t := range strings.Split(v, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				typesFilter = append(typesFilter, t)
			}
		}
	}

	req := &types.WikiGraphRequest{
		KnowledgeBaseID: kbID,
		Mode:            mode,
		Center:          center,
		Depth:           depth,
		Types:           typesFilter,
		Limit:           limit,
	}
	if h.memoryService != nil {
		req.FamiliarKnowledgeIDs = h.memoryService.FamiliarKnowledgeIDs(c.Request.Context())
	}

	graph, err := h.wikiService.GetGraph(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, graph)
}

// GetStats godoc
// @Summary      Get wiki statistics
// @Description  Returns aggregate statistics about the wiki
// @Tags         Wiki
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Success      200  {object}  types.WikiStats
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/stats [get]
func (h *WikiPageHandler) GetStats(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	stats, err := h.wikiService.GetStats(c.Request.Context(), kbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// ListIssues godoc
// @Summary      List wiki page issues
// @Description  List issues flagged on wiki pages with optional filtering
// @Tags         Wiki
// @Produce      json
// @Param        kb_id  path   string  true   "Knowledge base ID"
// @Param        slug   query  string  false  "Filter by page slug"
// @Param        status query  string  false  "Filter by status (pending, ignored, resolved)"
// @Success      200  {array}  types.WikiPageIssue
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/issues [get]
func (h *WikiPageHandler) ListIssues(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	slug := c.Query("slug")
	status := c.Query("status")

	issues, err := h.wikiService.ListIssues(c.Request.Context(), kbID, slug, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, issues)
}

// UpdateIssueStatus godoc
// @Summary      Update wiki page issue status
// @Description  Update the status of a flagged wiki page issue
// @Tags         Wiki
// @Accept       json
// @Produce      json
// @Param        kb_id    path  string  true  "Knowledge base ID"
// @Param        issue_id path  string  true  "Issue ID"
// @Param        status   body  object  true  "New status {'status': 'ignored'}"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  errors.AppError
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/issues/{issue_id}/status [put]
func (h *WikiPageHandler) UpdateIssueStatus(c *gin.Context) {
	_, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	issueID := secutils.SanitizeForLog(c.Param("issue_id"))
	if issueID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Issue ID is required"})
		return
	}

	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	validStatuses := map[string]bool{"pending": true, "ignored": true, "resolved": true}
	if !validStatuses[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status. Must be pending, ignored, or resolved"})
		return
	}

	if err := h.wikiService.UpdateIssueStatus(c.Request.Context(), issueID, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Issue status updated successfully"})
}

// SearchPages godoc
// @Summary      Search wiki pages
// @Description  Full-text search over wiki pages
// @Tags         Wiki
// @Produce      json
// @Param        kb_id  path   string  true   "Knowledge base ID"
// @Param        q      query  string  true   "Search query"
// @Param        limit  query  int     false  "Max results (default 10)"
// @Success      200  {array}  types.WikiPage
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/search [get]
func (h *WikiPageHandler) SearchPages(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Search query 'q' is required"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	pages, err := h.wikiService.SearchPages(c.Request.Context(), kbID, query, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"pages": pages})
}

// RebuildLinks godoc
// @Summary      Rebuild wiki links
// @Description  Re-parse all pages and rebuild bidirectional link references
// @Tags         Wiki
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Success      200  {object}  map[string]string
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/rebuild-links [post]
func (h *WikiPageHandler) RebuildLinks(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.wikiService.RebuildLinks(c.Request.Context(), kbID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Links rebuilt successfully"})
}

// Lint godoc
// @Summary      Run wiki lint
// @Description  Perform a comprehensive health check on the wiki
// @Tags         Wiki
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Success      200  {object}  service.WikiLintReport
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/lint [get]
func (h *WikiPageHandler) Lint(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	report, err := h.lintService.RunLint(c.Request.Context(), kbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, report)
}

// AutoFix godoc
// @Summary      Auto-fix wiki issues
// @Description  Automatically fix fixable wiki issues (broken links, etc.)
// @Tags         Wiki
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/auto-fix [post]
func (h *WikiPageHandler) AutoFix(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fixed, err := h.lintService.AutoFix(c.Request.Context(), kbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"fixed": fixed, "message": fmt.Sprintf("Auto-fixed %d issues", fixed)})
}
