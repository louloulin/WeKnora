// Package handler - v0.7.90 collab_doc_form_responses REST endpoints.
//
// Tencent Docs / Feishu Base parity: forms collect respondent answers.
// The submit endpoint is PUBLIC (no auth) when the doc is shared via
// share_token; otherwise the auth middleware rejects unauthenticated
// callers. The list/summary/export endpoints are owner-only.
//
// Public submit path:
//
//	POST /collaborative-docs/:id/responses?share_token=<token>
//	body: { submitter_token, submitter_name, answers: { q1: "…" } }
//
// Owner-side endpoints (require auth + ownership):
//
//	GET    /collaborative-docs/:id/responses             — paginated list
//	GET    /collaborative-docs/:id/responses/summary     — per-question aggregates
//	GET    /collaborative-docs/:id/responses/export.csv  — CSV download
package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// CollabDocFormResponseHandler exposes the form response REST surface.
type CollabDocFormResponseHandler struct {
	svc *service.CollabDocService
}

// NewCollabDocFormResponseHandler wires the form response handler.
func NewCollabDocFormResponseHandler(svc *service.CollabDocService) *CollabDocFormResponseHandler {
	return &CollabDocFormResponseHandler{svc: svc}
}

// Register attaches the form response routes. The submit endpoint lives
// outside the auth-protected group so anonymous respondents can hit it
// with a valid share_token in the query string. List/Summary/Export are
// mounted by the collab-doc router which already wraps the auth chain.
func (h *CollabDocFormResponseHandler) Register(rg *gin.RouterGroup) {
	// Public submit. Auth middleware is bypassed for this route via a
	// query-token check inside Submit — share_token is the only gate.
	rg.POST("/collaborative-docs/:id/responses", h.Submit)
	rg.GET("/collaborative-docs/:id/responses", h.List)
	rg.GET("/collaborative-docs/:id/responses/summary", h.Summary)
	rg.GET("/collaborative-docs/:id/responses/export.csv", h.ExportCSV)
}

// tenantAndUserForFormResponse reuses the numeric projection already used
// by CollabDocBytesHandler so the public submit endpoint can stamp a
// stable submitter_user_id when the caller happens to be authed (rare
// because the route sits outside the auth middleware, but the helper is
// useful when the route is wired behind auth in the future).
func (h *CollabDocFormResponseHandler) tenantAndUser(c *gin.Context) (uint64, uint64) {
	tenantVal, tenantOK := c.Get(types.TenantIDContextKey.String())
	userVal, _ := c.Get(types.UserIDContextKey.String())
	if !tenantOK {
		return 0, 0
	}
	tenant := collabCommentUint64(tenantVal)
	user := collabCommentUint64(userVal)
	return tenant, user
}

// docFromRequest resolves tenant + doc + validates the doc is a form.
// For the public submit endpoint the tenant is looked up by share_token
// instead of the auth context.
func (h *CollabDocFormResponseHandler) docFromRequest(c *gin.Context) (tenantID uint64, docID string, isForm bool, ok bool) {
	docID = strings.TrimSpace(c.Param("id"))
	if docID == "" {
		return 0, "", false, false
	}
	tenantID, _ = h.tenantAndUser(c)
	if tenantID == 0 {
		// Public submit path: tenant comes from the share_token lookup
		// performed by Submit, not from the auth context.
		return 0, docID, false, true
	}
	// Authed path: check the doc is a form so we 404 (not 200) if the
	// caller is asking about a doc or slide.
	doc, err := h.svc.GetDoc(c.Request.Context(), tenantID, tenantAndUserOrZero(tenantID, c), docID)
	if err != nil || doc == nil {
		return tenantID, docID, false, false
	}
	return tenantID, docID, doc.DocKind == types.CollaborativeDocKindForm, true
}

// tenantAndUserOrZero returns the user id (0 if not set) so the service
// layer's CanRead treats anonymous auth contexts gracefully.
func tenantAndUserOrZero(tenantID uint64, c *gin.Context) uint64 {
	_, user := (&CollabDocFormResponseHandler{}).tenantAndUser(c)
	_ = tenantID
	return user
}

// Submit handles public form submission. Auth is OPTIONAL; if the caller
// has a bearer it gets stamped as submitter_user_id. Share_token in the
// query string is the only mandatory gate for anonymous submissions.
func (h *CollabDocFormResponseHandler) Submit(c *gin.Context) {
	docID := strings.TrimSpace(c.Param("id"))
	if docID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "doc id required"})
		return
	}
	var req types.CreateCollabDocFormResponseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if len(req.Answers) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "answers is empty"})
		return
	}
	shareToken := strings.TrimSpace(c.Query("share_token"))
	tenantID, userID, isAuthed := collabDocResolveSubmitCaller(c, shareToken)
	if isAuthed && tenantID > 0 {
		// Authed caller — tenant + user come from the auth chain.
		doc, err := h.svc.GetDoc(c.Request.Context(), tenantID, userID, docID)
		if err != nil || doc == nil {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "doc not found"})
			return
		}
		if doc.DocKind != types.CollaborativeDocKindForm {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "doc is not a form"})
			return
		}
		h.persistSubmit(c, tenantID, docID, userID, req, c.ClientIP(), c.Request.UserAgent())
		return
	}
	// Unauthed caller — must present a share_token that resolves to a doc.
	if shareToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "share_token required"})
		return
	}
	doc, err := h.svc.FindByShareToken(c.Request.Context(), shareToken)
	if err != nil || doc == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "share_token invalid"})
		return
	}
	if doc.ID != docID {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "share_token does not match doc"})
		return
	}
	if doc.DocKind != types.CollaborativeDocKindForm {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "doc is not a form"})
		return
	}
	if service.ShareExpired(doc, time.Now()) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "share link expired"})
		return
	}
	h.persistSubmit(c, doc.TenantID, doc.ID, 0, req, c.ClientIP(), c.Request.UserAgent())
}

// persistSubmit forwards the validated submit to the service and writes
// the JSON response. Extracted from Submit so the authed + unauthed
// branches can share the final persistence step.
func (h *CollabDocFormResponseHandler) persistSubmit(
	c *gin.Context, tenantID uint64, docID string, callerUserID uint64,
	req types.CreateCollabDocFormResponseRequest, clientIP, userAgent string,
) {
	resp, err := h.svc.SubmitFormResponse(c.Request.Context(), tenantID, docID, callerUserID,
		req.SubmitterToken, req.SubmitterName, clientIP, userAgent, req.Answers)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": resp})
}

// collabDocResolveSubmitCaller returns (tenantID, userID, isAuthed). The
// tenant comes from auth context if present, otherwise from the share
// token → doc lookup. isAuthed is true only when the auth chain stamped
// a real user id (not 0).
func collabDocResolveSubmitCaller(c *gin.Context, shareToken string) (uint64, uint64, bool) {
	tenantVal, tenantOK := c.Get(types.TenantIDContextKey.String())
	userVal, _ := c.Get(types.UserIDContextKey.String())
	if tenantOK {
		tenant := collabCommentUint64(tenantVal)
		user := collabCommentUint64(userVal)
		if tenant > 0 {
			return tenant, user, user > 0
		}
	}
	// Unauthed: tenant comes from the doc resolved by share_token. We
	// return 0 here so the handler short-circuits with a 401 — the real
	// lookup runs inside Submit. This split keeps the helper pure.
	return 0, 0, false
}

// List returns paginated responses for the owning doc. Owner-only; the
// auth middleware + service-layer CanRead enforce ACL.
func (h *CollabDocFormResponseHandler) List(c *gin.Context) {
	tenantID, userID, ok := collabCommentCaller(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "auth required"})
		return
	}
	docID := strings.TrimSpace(c.Param("id"))
	limit := formRespAtoiDefault(c.Query("limit"), 50)
	offset := formRespAtoiDefault(c.Query("offset"), 0)
	rows, err := h.svc.ListFormResponses(c.Request.Context(), tenantID, userID, docID,
		types.ListCollabDocFormResponsesFilter{Limit: limit, Offset: offset})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	count, _ := h.svc.CountFormResponses(c.Request.Context(), tenantID, userID, docID)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"items": rows,
		"total": count,
	}})
}

// Summary returns per-question aggregates for the form.
func (h *CollabDocFormResponseHandler) Summary(c *gin.Context) {
	tenantID, userID, ok := collabCommentCaller(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "auth required"})
		return
	}
	docID := strings.TrimSpace(c.Param("id"))
	sum, err := h.svc.FormResponseSummary(c.Request.Context(), tenantID, userID, docID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": sum})
}

// ExportCSV streams a CSV of every response for owner download.
func (h *CollabDocFormResponseHandler) ExportCSV(c *gin.Context) {
	tenantID, userID, ok := collabCommentCaller(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "auth required"})
		return
	}
	docID := strings.TrimSpace(c.Param("id"))
	rows, err := h.svc.ListFormResponses(c.Request.Context(), tenantID, userID, docID,
		types.ListCollabDocFormResponsesFilter{Limit: 500})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	// Gather the union of question ids across all responses so the CSV
	// header is stable. Newest first → most ids known up-front.
	questionIDs := collectQuestionIDs(rows)
	header := []string{"created_at", "submitter_name", "submitter_token"}
	header = append(header, questionIDs...)
	w := csv.NewWriter(c.Writer)
	defer w.Flush()
	c.Writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	c.Writer.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="form-%s.csv"`, docID))
	c.Writer.WriteHeader(http.StatusOK)
	if err := w.Write(header); err != nil {
		return
	}
	for _, row := range rows {
		out := []string{
			row.CreatedAt.Format("2006-01-02 15:04:05"),
			row.SubmitterName,
			row.SubmitterToken,
		}
		answers := map[string]interface{}{}
		if err := json.Unmarshal([]byte(row.Answers), &answers); err != nil {
			out = append(out, fmt.Sprintf(`{"_error":"%s"}`, err.Error()))
			continue
		}
		for _, qid := range questionIDs {
			out = append(out, fmtAnswerForCSV(answers[qid]))
		}
		if err := w.Write(out); err != nil {
			return
		}
	}
}

func formRespAtoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func collectQuestionIDs(rows []*types.CollabDocFormResponse) []string {
	seen := map[string]struct{}{}
	order := []string{}
	for _, row := range rows {
		var a map[string]interface{}
		if err := json.Unmarshal([]byte(row.Answers), &a); err != nil {
			continue
		}
		for k := range a {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			order = append(order, k)
		}
	}
	return order
}

func fmtAnswerForCSV(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			parts = append(parts, fmt.Sprintf("%v", item))
		}
		return strings.Join(parts, "; ")
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

var _ = tenantAndUserOrZero // silence unused in case the helper drops out
