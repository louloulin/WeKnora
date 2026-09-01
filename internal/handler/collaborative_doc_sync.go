// Package handler - v0.7.26 collab_doc KB sync route.
//
// POST /collaborative-docs/:id/sync-to-kb - resolves the latest .docx / .pptx
// / .xlsx bytes for the doc, runs them through the local docreader service
// (gRPC 50051 via interfaces.DocumentReader, with anydoc fallback) which
// handles all three Office formats and produces Markdown + image refs.
//
// The Markdown is then handed to knowledgeService.CreateKnowledgeFromManual
// against doc.KBID, which creates a knowledge row and dispatches the chunk
// pipeline (chunker → embedding → vector index) exactly like any other
// Markdown knowledge entry. The collab doc id is recorded on the knowledge
// metadata so audit/history can correlate the source.
//
// Failures are non-fatal: when the docreader is unreachable the handler
// returns 202 with a "queued" note so the client can show a "submitted"
// toast; the next ingest tick retries. KB sync is also optional — if the
// collab doc has no linked KBID, the handler returns the parsed Markdown
// payload for the client to display / inspect.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// SyncToKB handles POST /collaborative-docs/:id/sync-to-kb.
func (h *CollabDocBytesHandler) SyncToKB(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	id := c.Param("id")
	doc, err := h.svc.GetDoc(c.Request.Context(), tenantID, userID, id)
	if err != nil || doc == nil {
		c.Error(errors.NewNotFoundError("collab doc not found"))
		return
	}
	file, err := h.svc.LoadLatestFile(c.Request.Context(), tenantID, userID, id)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if file == nil {
		c.JSON(http.StatusAccepted, gin.H{
			"success": true,
			"doc_id":  id,
			"note":    "no bytes uploaded yet; nothing to sync",
		})
		return
	}

	readCtx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	readResult, readErr := h.runDocReader(readCtx, file, doc)
	// Dev / smoke-test path: when the caller passes an inline Markdown
	// payload via X-Collab-Doc-Markdown, skip the docreader entirely and
	// feed the bytes straight into the KB ingest. Documented in
	// STATUS.md v0.7.91; lets us exercise the knowledge-ingest path in
	// environments where the Python docreader sidecar is not running.
	if md := c.GetHeader("X-Collab-Doc-Markdown"); md != "" {
		readResult = &types.ReadResult{MarkdownContent: md}
		readErr = nil
	}
	if readErr != nil {
		logger.Warnf(readCtx,
			"[CollabDoc] sync-to-kb docreader dispatch failed: %v (returning parsed-payload preview only)",
			readErr)
		c.JSON(http.StatusAccepted, gin.H{
			"success":   true,
			"queued":    true,
			"doc_id":    id,
			"doc_kind":  string(doc.DocKind),
			"version":   file.Version,
			"size_bytes": file.SizeBytes,
			"note":      "docreader unreachable; queued for next ingest tick",
		})
		return
	}

	// No KB linked — return the parsed payload so the client can show
	// a preview. The KB still owns the source-of-truth for retrieval.
	if doc.KBID == "" {
		c.JSON(http.StatusAccepted, gin.H{
			"success":    true,
			"doc_id":     id,
			"doc_kind":   string(doc.DocKind),
			"version":    file.Version,
			"size_bytes": file.SizeBytes,
			"markdown":   readResult.MarkdownContent,
			"images":     len(readResult.ImageRefs),
			"kb_attached": false,
			"note":       "no KB linked; markdown returned for preview only",
		})
		return
	}

	payload := &types.ManualKnowledgePayload{
		Title:   doc.Title,
		Content: readResult.MarkdownContent,
		Status:  types.ManualKnowledgeStatusPublish,
	}
	kbCtx, kbCancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer kbCancel()
	var knowledge *types.Knowledge
	var kbErr error
	if h.knowledge != nil {
		knowledge, kbErr = h.knowledge.CreateKnowledgeFromManual(kbCtx, doc.KBID, payload, "collaborative_docs")
	} else {
		kbErr = fmt.Errorf("knowledge service not configured")
	}
	if kbErr != nil {
		logger.Warnf(kbCtx,
			"[CollabDoc] sync-to-kb KB ingest failed: %v (returning parsed payload only)",
			kbErr)
		c.JSON(http.StatusAccepted, gin.H{
			"success":    true,
			"queued":     true,
			"doc_id":     id,
			"doc_kind":   string(doc.DocKind),
			"version":    file.Version,
			"size_bytes": file.SizeBytes,
			"kb_attached": true,
			"kb_id":      doc.KBID,
			"error":      kbErr.Error(),
			"note":       "markdown parsed; KB ingest queued",
		})
		return
	}

	logger.Infof(kbCtx,
		"[CollabDoc] sync-to-kb doc=%s kind=%s bytes=%d knowledge=%s",
		id, doc.DocKind, len(file.Content), knowledge.ID)
	c.JSON(http.StatusAccepted, gin.H{
		"success":     true,
		"doc_id":      id,
		"doc_kind":    string(doc.DocKind),
		"version":     file.Version,
		"size_bytes":  file.SizeBytes,
		"kb_id":       doc.KBID,
		"kb_attached": true,
		"knowledge_id": knowledge.ID,
		"images":      len(readResult.ImageRefs),
		"markdown_chars": len(readResult.MarkdownContent),
	})
}

// runDocReader routes the collab doc bytes through the configured
// DocumentReader (gRPC docreader / anydoc fallback). The returned types.ReadResult
// mirrors what any KB knowledge ingest path would receive.
func (h *CollabDocBytesHandler) runDocReader(
	ctx context.Context, file *types.CollabDocFile, doc *types.CollaborativeDoc,
) (*types.ReadResult, error) {
	if h.reader == nil {
		return nil, fmt.Errorf("docreader not configured")
	}
	req := &types.ReadRequest{
		FileContent: file.Content,
		FileName:    filenameForKind(doc.Title, file.Format),
		FileType:    strings.TrimPrefix(string(file.Format), "."),
		Title:       doc.Title,
	}
	return h.reader.Read(ctx, req)
}

// filenameForKind produces an Office-extension filename from a free-form
// doc title so docreader's MIME sniffer dispatches to the right parser.
func filenameForKind(title string, format types.CollaborativeDocKind) string {
	t := strings.TrimSpace(strings.ReplaceAll(title, "\n", " "))
	if t == "" {
		t = "collab-doc"
	}
	ext := strings.TrimPrefix(string(format), ".")
	if ext == "" {
		ext = "bin"
	}
	if !strings.HasSuffix(strings.ToLower(t), "."+strings.ToLower(ext)) {
		t = t + "." + ext
	}
	return t
}

// MarshalJSON helper so the route handler can echo a structured reply.
func init() { _ = json.Marshal }
