// Package handler — v0.7.26 collab_doc binary upload / download handlers.
//
// Endpoints:
//
//	POST   /collaborative-docs/:id/upload    multipart/form-data; file field
//	GET    /collaborative-docs/:id/download  raw bytes; proper Content-Type
//
// These pair with the Yjs CRDT channel: the editor pulls the latest bytes
// on open (download), mutates them locally, and POSTs a fresh version on
// save (upload). The Yjs snapshot is independently persisted by the WS
// handler.
package handler

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// CollabDocBytesHandler is the binary REST surface for collab docs.
type CollabDocBytesHandler struct {
	svc *service.CollabDocService
}

// NewCollabDocBytesHandler wires the binary handler.
func NewCollabDocBytesHandler(svc *service.CollabDocService) *CollabDocBytesHandler {
	return &CollabDocBytesHandler{svc: svc}
}

// Mount attaches the binary routes onto an authenticated v1 group.
func (h *CollabDocBytesHandler) Mount(rg *gin.RouterGroup) {
	rg.POST("/collaborative-docs/:id/upload", h.Upload)
	rg.GET("/collaborative-docs/:id/download", h.Download)
	rg.GET("/collaborative-docs/:id/download/:version", h.DownloadVersion)
	rg.GET("/collaborative-docs/:id/files", h.ListFiles)
	rg.POST("/collaborative-docs/:id/sync-to-kb", h.SyncToKB)
	// Public read-only download by share token (no auth).
	rg.GET("/collaborative-docs/share/:token/download", h.ShareDownload)
	rg.GET("/collaborative-docs/share/:token/form-schema", h.ShareFormSchema)
}

// Upload handles POST /collaborative-docs/:id/upload.
//
// Accepts a multipart/form-data body with a single "file" field. Filename
// extension is used to pick the doc_kind (allowed only when it matches the
// persisted doc_kind). Version is auto-incremented when omitted.
func (h *CollabDocBytesHandler) Upload(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	docID := c.Param("id")
	if docID == "" {
		c.Error(errors.NewBadRequestError("missing doc id"))
		return
	}
	doc, err := h.svc.GetDoc(c.Request.Context(), tenantID, userID, docID)
	if err != nil || doc == nil {
		c.Error(errors.NewNotFoundError("collab doc not found"))
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		c.Error(errors.NewBadRequestError("missing 'file' multipart field: " + err.Error()))
		return
	}
	// Cap upload size at 64 MiB so a malicious client can't OOM the service.
	const maxUploadBytes = 64 << 20
	if fh.Size > maxUploadBytes {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("file too large: %d > %d", fh.Size, maxUploadBytes)))
		return
	}
	src, err := fh.Open()
	if err != nil {
		c.Error(errors.NewInternalServerError("open uploaded file: " + err.Error()))
		return
	}
	defer src.Close()
	content, err := io.ReadAll(io.LimitReader(src, maxUploadBytes+1))
	if err != nil {
		c.Error(errors.NewInternalServerError("read uploaded file: " + err.Error()))
		return
	}
	if len(content) == 0 {
		c.Error(errors.NewBadRequestError("uploaded file is empty"))
		return
	}
	// Validate kind by extension; reject mismatches.
	format := kindFromFilename(fh.Filename)
	if format == "" {
		format = doc.DocKind
	}
	if format != doc.DocKind {
		c.Error(errors.NewBadRequestError(fmt.Sprintf(
			"uploaded file extension does not match doc_kind: kind=%s extension=%s",
			doc.DocKind, format,
		)))
		return
	}
	// Compute sha256 for audit / dedup.
	sum := sha256.Sum256(content)
	hexSum := hex.EncodeToString(sum[:])
	logger.Infof(c.Request.Context(), "[CollabDoc] upload doc=%s kind=%s bytes=%d sha256=%s",
		docID, format, len(content), hexSum)
	row, err := h.svc.SaveFile(c.Request.Context(), tenantID, userID, docID, types.CollabDocFileUpsert{
		TenantID: tenantID,
		DocID:    docID,
		Format:   format,
		Content:  content,
		Version:  0, // auto-increment
	})
	if err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	// v0.7.30 — record audit event. Non-blocking; middleware swallows
	// errors via service.RecordAudit.
	payload := fmt.Sprintf(`{"format":%q,"size_bytes":%d,"sha256":%q,"version":%d}`,
		string(format), row.SizeBytes, hexSum, row.Version)
	h.svc.RecordAudit(c.Request.Context(), types.RecordAuditRequest{
		TenantID:    tenantID,
		DocID:       docID,
		ActorUserID: userID,
		Action:      types.AuditActionUpload,
		Target:      fmt.Sprintf("v%d", row.Version),
		Payload:     payload,
		IP:          c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
	})
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"id":         row.ID,
			"doc_id":     row.DocID,
			"format":     row.Format,
			"size_bytes": row.SizeBytes,
			"sha256":     hexSum,
			"version":    row.Version,
			"created_at": row.CreatedAt,
		},
	})
}

// Download handles GET /collaborative-docs/:id/download.
func (h *CollabDocBytesHandler) Download(c *gin.Context) {
	h.download(c, 0)
}

// DownloadVersion handles GET /collaborative-docs/:id/download/:version.
func (h *CollabDocBytesHandler) DownloadVersion(c *gin.Context) {
	var v int
	if _, err := fmt.Sscanf(c.Param("version"), "%d", &v); err != nil || v <= 0 {
		c.Error(errors.NewBadRequestError("invalid version"))
		return
	}
	h.download(c, v)
}

func (h *CollabDocBytesHandler) download(c *gin.Context, version int) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	docID := c.Param("id")
	doc, err := h.svc.GetDoc(c.Request.Context(), tenantID, userID, docID)
	if err != nil || doc == nil {
		c.Error(errors.NewNotFoundError("collab doc not found"))
		return
	}
	var row *types.CollabDocFile
	if version > 0 {
		row, err = h.svc.LoadFileByVersion(c.Request.Context(), tenantID, userID, docID, version)
	} else {
		row, err = h.svc.LoadLatestFile(c.Request.Context(), tenantID, userID, docID)
	}
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if row == nil {
		c.Error(errors.NewNotFoundError("no file uploaded yet"))
		return
	}
	mime := mimeForKind(row.Format)
	filename := fmt.Sprintf("%s-v%d%s", sanitizeFilename(doc.Title), row.Version, extForKind(row.Format))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("X-Collab-Doc-Version", fmt.Sprintf("%d", row.Version))
	sum := sha256.Sum256(row.Content)
	c.Header("X-Collab-Doc-SHA256", hex.EncodeToString(sum[:]))
	c.Data(http.StatusOK, mime, row.Content)
}

func (h *CollabDocBytesHandler) tenantAndUser(c *gin.Context) (uint64, uint64, bool) {
	t := c.GetUint64(types.TenantIDContextKey.String())
	// v0.7.73 — UserIDContextKey is set by middleware to user.ID (string UUID);
	// fall back to a stable sha256 hash for handlers that look up by numeric user ID.
	var u uint64
	if v, ok := c.Get(types.UserIDContextKey.String()); ok {
		switch x := v.(type) {
		case uint64:
			u = x
		case string:
			h := sha256.Sum256([]byte(x))
			u = binary.BigEndian.Uint64(h[:8]) &^ (uint64(1) << 63)
		}
	}
	if t == 0 || u == 0 {
		c.Error(errors.NewUnauthorizedError("missing tenant/user context"))
		return 0, 0, false
	}
	return t, u, true
}

func kindFromFilename(name string) types.CollaborativeDocKind {
	ext := strings.ToLower(filepath.Ext(name))
	lower := strings.ToLower(name)
	// v0.7.73 — form JSON: accept `.form.json` and plain `.json` for form docs.
	if ext == ".form.json" || (ext == ".json" && strings.HasSuffix(lower, ".form.json")) {
		return types.CollaborativeDocKindForm
	}
	switch ext {
	case ".docx":
		return types.CollaborativeDocKindDoc
	case ".pptx":
		return types.CollaborativeDocKindSlide
	case ".xlsx":
		return types.CollaborativeDocKindSheet
	case ".json":
		return types.CollaborativeDocKindForm
	}
	return ""
}

func extForKind(k types.CollaborativeDocKind) string {
	switch k {
	case types.CollaborativeDocKindDoc:
		return ".docx"
	case types.CollaborativeDocKindSlide:
		return ".pptx"
	case types.CollaborativeDocKindSheet:
		return ".xlsx"
	case types.CollaborativeDocKindForm:
		return ".form.json"
	}
	return ""
}

func mimeForKind(k types.CollaborativeDocKind) string {
	switch k {
	case types.CollaborativeDocKindDoc:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case types.CollaborativeDocKindSlide:
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case types.CollaborativeDocKindSheet:
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case types.CollaborativeDocKindForm:
		return "application/json"
	}
	return "application/octet-stream"
}

func sanitizeFilename(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			out = append(out, r)
		} else if r == ' ' {
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "collab-doc"
	}
	return string(out)
}

// ShareDownload handles GET /collaborative-docs/share/:token/download.
//
// Public read-only download gated on the doc's share_token. Returns 404
// when the token does not resolve to any doc, 403 when the doc isn't
// marked shareable, and the file bytes otherwise. The response sets
// Content-Disposition: inline so browsers can preview without a forced
// download dialog.
func (h *CollabDocBytesHandler) ShareDownload(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.Error(errors.NewBadRequestError("missing share token"))
		return
	}
	doc, err := h.svc.FindByShareToken(c.Request.Context(), token)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if doc == nil {
		c.Error(errors.NewNotFoundError("share link not found"))
		return
	}
	if doc.Visibility != "public" && doc.Visibility != "shared" {
		c.Error(errors.NewForbiddenError("share link disabled"))
		return
	}
	// v0.7.38 Build #46.x — enforce expiry + password protection.
	if service.ShareExpired(doc, time.Now()) {
		c.Error(errors.NewNotFoundError("share link expired"))
		return
	}
	if !service.VerifySharePassword(doc, c.GetHeader("X-Share-Password")) {
		c.Header("WWW-Authenticate", `Share password="X-Share-Password"`)
		c.Error(errors.NewForbiddenError("share password required or wrong"))
		return
	}
	tenantID := doc.TenantID
	userID := doc.OwnerUserID
	file, err := h.svc.LoadLatestFile(c.Request.Context(), tenantID, userID, doc.ID)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if file == nil {
		c.Error(errors.NewNotFoundError("no file uploaded yet"))
		return
	}
	filename := fmt.Sprintf("%s-v%d%s", sanitizeFilename(doc.Title), file.Version, extForKind(file.Format))
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	c.Data(http.StatusOK, mimeForKind(file.Format), file.Content)
}

// ListFiles handles GET /collaborative-docs/:id/files.
//
// Returns a metadata-only list of every uploaded version (version number,
// size, sha256, created_at) — no bytes. Drives the "版本历史" panel in
// the DOC editor.
func (h *CollabDocBytesHandler) ListFiles(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	docID := c.Param("id")
	doc, err := h.svc.GetDoc(c.Request.Context(), tenantID, userID, docID)
	if err != nil || doc == nil {
		c.Error(errors.NewNotFoundError("collab doc not found"))
		return
	}
	rows, err := h.svc.ListFiles(c.Request.Context(), tenantID, docID)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	out := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]interface{}{
			"id":         r.ID,
			"doc_id":     r.DocID,
			"format":     r.Format,
			"version":    r.Version,
			"size_bytes": r.SizeBytes,
			"sha256":     r.SHA256,
			"created_at": r.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// ShareFormSchema returns the form metadata plus its latest JSON schema to
// an anonymous responder. It avoids forcing the public page to infer the
// document id from Content-Disposition filenames. The same share-token
// checks as ShareDownload are applied before the schema is returned.
func (h *CollabDocBytesHandler) ShareFormSchema(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		c.Error(errors.NewBadRequestError("missing share token"))
		return
	}
	doc, err := h.svc.FindByShareToken(c.Request.Context(), token)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if doc == nil || (doc.Visibility != "public" && doc.Visibility != "shared") {
		c.Error(errors.NewNotFoundError("share link not found"))
		return
	}
	if service.ShareExpired(doc, time.Now()) {
		c.Error(errors.NewNotFoundError("share link expired"))
		return
	}
	if !service.VerifySharePassword(doc, c.GetHeader("X-Share-Password")) {
		c.Header("WWW-Authenticate", `Share password="X-Share-Password"`)
		c.Error(errors.NewForbiddenError("share password required or wrong"))
		return
	}
	if doc.DocKind != types.CollaborativeDocKindForm {
		c.Error(errors.NewBadRequestError("shared document is not a form"))
		return
	}
	file, err := h.svc.LoadLatestFile(c.Request.Context(), doc.TenantID, doc.OwnerUserID, doc.ID)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if file == nil || len(file.Content) == 0 {
		c.Error(errors.NewNotFoundError("form schema not found"))
		return
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(file.Content, &schema); err != nil {
		c.Error(errors.NewInternalServerError("invalid form schema"))
		return
	}
	schema["doc_id"] = doc.ID
	schema["title"] = doc.Title
	schema["doc_kind"] = string(doc.DocKind)
	schema["share_token"] = token
	c.JSON(http.StatusOK, gin.H{"success": true, "data": schema})
}
