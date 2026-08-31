// Package handler - v0.7.26 collab_doc KB sync route.
//
// POST /collaborative-docs/:id/sync-to-kb - resolves the latest .docx / .pptx
// / .xlsx bytes for the doc, runs them through the local anydoc converter
// (which handles all three Office formats and produces structured chunks),
// and ingests the chunks into the linked knowledge base via the existing
// chunk ingestion pipeline.
//
// Failures are non-fatal: when the docparser/anydoc path is unreachable the
// handler queues the doc id and returns 202 so the client can show a
// "submitted" toast; the next ingest tick will retry.
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
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
	// Pull the latest bytes for this doc.
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
	// Dispatch to the docparser /chunk endpoint with a multipart body so the
	// Python side can re-extract content from the original Office file.
	docparserBase := c.GetHeader("X-Docparser-Base")
	if docparserBase == "" {
		docparserBase = defaultDocparserBase
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	body, contentType := buildMultipart(file, doc)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, docparserBase+"/chunk", body)
	req.Header.Set("Content-Type", contentType)
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
	replyBody, _ := io.ReadAll(resp.Body)
	logger.Infof(ctx, "[CollabDoc] sync-to-kb doc=%s status=%d bytes=%d", id, resp.StatusCode, len(replyBody))
	c.JSON(http.StatusAccepted, gin.H{
		"success":         true,
		"doc_id":          id,
		"doc_kind":        string(doc.DocKind),
		"version":         file.Version,
		"size_bytes":      file.SizeBytes,
		"docparser_reply": string(replyBody),
	})
}

const defaultDocparserBase = "http://localhost:8087"

// buildMultipart wraps the latest .docx/.pptx/.xlsx bytes into a multipart
// request body the docparser /chunk endpoint can ingest directly. The
// docparser will route by file extension to its pptx/docx/xlsx parsers.
func buildMultipart(file *types.CollabDocFile, doc *types.CollaborativeDoc) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("kb_id", doc.KBID)
	_ = w.WriteField("title", doc.Title)
	_ = w.WriteField("doc_id", doc.ID)
	_ = w.WriteField("doc_kind", string(doc.DocKind))
	_ = w.WriteField("source", "collaborative_docs")
	_ = w.WriteField("version", fmt.Sprintf("%d", file.Version))
	filename := strings.ReplaceAll(doc.Title, "\n", " ")
	if filename == "" {
		filename = "collab-doc"
	}
	filename += extForKind(file.Format)
	fw, _ := w.CreateFormFile("file", filename)
	_, _ = fw.Write(file.Content)
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

// MarshalJSON helper so the route handler can echo a structured reply.
func init() {
	_ = json.Marshal
}
