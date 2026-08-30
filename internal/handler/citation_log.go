package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// Build #30 — chat pipeline citation access tracking handler.
//
// The chat answer pipeline (Build #30 B3) rewrites the LLM output so every
// `<kb>` reference becomes a `[[cite:N]]` token, and stores the matching
// CitationIndex on `chatManage.CitationIndex`. The frontend then renders
// each token as a clickable `[来源 N]` chip; clicking the chip — or the
// answer-completion event when citations are enabled — posts here so we
// can leave one audit_logs row per access.
//
// Why a separate endpoint instead of folding the write into the chat
// completion event:
//
//  1. Audit writes must survive partial streaming failures. If the
//     WebSocket drops mid-answer the client still has a citation_index
//     to report, and the backend can persist it independently of the
//     chat stream's own state.
//  2. B25's correlation_id machinery expects a per-request audit row,
//     not a stream-bound one. Folding into the completion event would
//     require the stream goroutine to own its own X-Request-ID, which
//     complicates the per-turn correlation story.
//
// Async semantics (D9): the write happens in a goroutine spawned inside
// the handler. The handler returns 200 the moment the body validates —
// a transient audit outage must never block the chat UX. Failures are
// warn-logged with the request fields so operators can spot partial
// outages via Prom counters (Build #23 audit_sink metrics).
type CitationLogHandler struct {
	auditSvc interfaces.AuditLogService
}

// NewCitationLogHandler builds the handler with its single dependency.
// auditSvc may be nil in test rigs that exercise only request validation
// — the handler degrades to a no-op rather than panicking, matching the
// pattern used by WikiPageHandler when wikiAuditSvc is absent.
func NewCitationLogHandler(auditSvc interfaces.AuditLogService) *CitationLogHandler {
	return &CitationLogHandler{auditSvc: auditSvc}
}

// CitationLogRequest is the wire shape. Every field except `Title` is
// required; the handler returns 400 if any of them is empty / zero.
// `Title` is best-effort preview metadata — the chunk title captured at
// the moment the chat answer was rendered, useful in audit listings.
type CitationLogRequest struct {
	KnowledgeBaseID string `json:"kb_id"`
	ChunkID         string `json:"chunk_id"`
	SourceMessageID string `json:"source_message_id"`
	CitationIndex   int    `json:"citation_index"`
	Title           string `json:"title,omitempty"`
}

// LogCitationAccess is the POST handler. Route binding lives in
// router.RegisterChatRoutes.
//
// Behaviour contract:
//   - Body must parse as CitationLogRequest; otherwise 400.
//   - Required fields must be non-empty / non-zero; otherwise 400.
//   - On success returns 200 immediately and queues an audit write
//     in a goroutine; the goroutine never errors back to the caller.
//   - When auditSvc is nil (test wiring), the handler still returns
//     200 and logs a debug line — never panic, never block.
func (h *CitationLogHandler) LogCitationAccess(c *gin.Context) {
	var req CitationLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body: " + err.Error()})
		return
	}
	if req.KnowledgeBaseID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb_id is required"})
		return
	}
	if req.ChunkID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chunk_id is required"})
		return
	}
	if req.SourceMessageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_message_id is required"})
		return
	}
	if req.CitationIndex <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "citation_index must be a positive 1-based number"})
		return
	}

	// Tenant comes from the auth middleware, never from the body. We do
	// not trust a tenant id baked into the payload — that would let a
	// client write audit rows into a sibling tenant.
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant context"})
		return
	}

	if h.auditSvc == nil {
		// Test-only wiring: still 200 so the chat UX does not regress when
		// audit is intentionally disabled. The Build #23 audit_sink metrics
		// canary catches the disabled case at scrape time.
		logger.Debugf(c.Request.Context(),
			"[citation_log] audit service unavailable; skipping write chunk_id=%s citation_index=%d",
			req.ChunkID, req.CitationIndex)
		c.JSON(http.StatusOK, gin.H{"status": "skipped"})
		return
	}

	actorID, _ := types.UserIDFromContext(c.Request.Context())
	actorRole := types.TenantRoleFromContext(c.Request.Context())

	// Build the audit row. scope_type=knowledge_base + scope_id=kb_id
	// keeps the row inside the existing WikiAuditSourceActivity projection
	// (Build #24) — operators viewing the KB audit feed will see
	// "citation_accessed" events interleaved with wiki mutations. session
	// id and message id go into the Details payload so cross-turn
	// correlation (Build #30 A9) joins on the JSON.
	details := map[string]any{
		"chunk_id":          req.ChunkID,
		"source_message_id": req.SourceMessageID,
		"citation_index":    req.CitationIndex,
		"kb_id":             req.KnowledgeBaseID,
	}
	if req.Title != "" {
		details["title"] = req.Title
	}
	detailJSON, _ := json.Marshal(details)

	entry := &types.AuditLog{
		TenantID:      tenantID,
		ActorUserID:   actorID,
		ActorRole:     string(actorRole),
		Action:        types.AuditActionChatCitationAccessed,
		ScopeType:     "knowledge_base",
		ScopeID:       req.KnowledgeBaseID,
		TargetType:    "citation",
		TargetID:      req.ChunkID,
		Outcome:       types.AuditOutcomeSuccess,
		Details:       types.JSON(detailJSON),
		RequestPath:   c.FullPath(),
		RequestMethod: c.Request.Method,
	}

	// Detach the request context so the goroutine survives the
	// handler return. CloneContext preserves X-Request-ID (Build #25
	// correlation_id) and the active OTel span across the rebuild so
	// the audit row joins the same trace as the original chat turn.
	goroutineCtx := logger.CloneContext(c.Request.Context())

	// Fire-and-forget. We copy the entry by pointer (the struct is
	// read-only inside the goroutine) and the goroutine outlives the
	// request scope. auditLogService.Log already stamps
	// entry.CorrelationID from CorrelationIDFromContext when the entry
	// value is empty (B25), so the audit row will carry the same
	// X-Request-ID as the inbound POST.
	go func(ctx context.Context, e *types.AuditLog, citationIndex int) {
		if err := h.auditSvc.Log(ctx, e); err != nil {
			logger.Warnf(ctx,
				"[citation_log] audit write failed kb_id=%s chunk_id=%s citation_index=%d: %v",
				e.ScopeID, e.TargetID, citationIndex, err)
		}
	}(goroutineCtx, entry, req.CitationIndex)

	c.JSON(http.StatusOK, gin.H{
		"status":            "accepted",
		"chunk_id":          req.ChunkID,
		"citation_index":    req.CitationIndex,
		"source_message_id": req.SourceMessageID,
	})
}
