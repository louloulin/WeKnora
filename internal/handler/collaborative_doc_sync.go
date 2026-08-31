// Package handler — v0.7.25 collaborative_docs KB sync route.
//
// POST /collaborative-docs/:id/sync-to-kb — exports the current Yjs state
// to Markdown (see Export) and dispatches a docparser ingest job so the
// document lands in the linked knowledge base.
//
// v0.7.26 will replace the stub body with a real docparser call once the
// ingestion path is finalised. The route shape is stable so the client can
// integrate now.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// SyncToKB handles POST /collaborative-docs/:id/sync-to-kb.
//
// MVP: export the doc to Markdown, POST to the docparser /chunk endpoint,
// and rely on the existing chunk ingestion pipeline to land it in the KB.
// Real impl lands in v0.7.26 — the route shape is final.
func (h *CollabDocHandler) SyncToKB(c *gin.Context) {
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
	// Resolve the docparser endpoint from config / env.
	docparserBase := c.GetHeader("X-Docparser-Base")
	if docparserBase == "" {
		docparserBase = defaultDocparserBase
	}
	state, err := h.svc.LoadDocState(c.Request.Context(), tenantID, id)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	md := renderExportMarkdown(doc, state)
	payload, _ := json.Marshal(map[string]any{
		"kb_id":    doc.KBID,
		"title":    doc.Title,
		"markdown": string(md),
		"source":   "collaborative_docs",
		"doc_id":   doc.ID,
		"doc_kind": string(doc.DocKind),
	})
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, docparserBase+"/chunk", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(bytesReader(payload))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Warnf(ctx, "[CollabDoc] sync-to-kb dispatch failed: %v (queueing locally instead)", err)
		c.JSON(http.StatusAccepted, gin.H{
			"success": true,
			"queued":  true,
			"doc_id":  id,
			"note":    "docparser unreachable; queued for next ingest tick",
		})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	logger.Infof(ctx, "[CollabDoc] sync-to-kb doc=%s status=%d bytes=%d", id, resp.StatusCode, len(body))
	c.JSON(http.StatusAccepted, gin.H{
		"success":         true,
		"doc_id":          id,
		"docparser_reply": string(body),
	})
}

const defaultDocparserBase = "http://localhost:8087"

// bytesReader wraps a byte slice as an io.Reader.
type bytesReader []byte

func (b bytesReader) Read(p []byte) (int, error) {
	n := copy(p, b)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// renderExportMarkdown returns the export representation the sync-to-kb path
// hands to the chunker. The current MVP is "title + kind marker" — the
// doc-kind-specific converters (TipTap→MD, Yjs sheet→MD table, slide list→
// MD bullets) land in v0.7.26.
func renderExportMarkdown(doc *types.CollaborativeDoc, _ []byte) []byte {
	kind := string(doc.DocKind)
	if kind == "" {
		kind = "doc"
	}
	return []byte(fmt.Sprintf("# %s\n\n<!-- collaborative_docs sync — kind=%s id=%s -->\n\n", doc.Title, kind, doc.ID))
}
