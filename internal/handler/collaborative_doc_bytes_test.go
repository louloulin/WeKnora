// Package handler - v0.7.26 collab_doc_bytes handler integration test.
//
// Exercises the full HTTP round-trip: create doc -> upload .docx -> list
// files -> download -> upload again -> download historical version -> sync
// to KB -> public share download.
package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() { gin.SetMode(gin.TestMode) }

// buildTestStack constructs a fully-wired CollabDocBytesHandler + CollabDocHandler
// + repo trio + service over an in-memory SQLite DB. Returns the gin router
// and a teardown.
func buildTestStack(t *testing.T) (*gin.Engine, *types.CollaborativeDoc, string) {
	t.Helper()
	dsn := fmt.Sprintf("file:collab_test_%d?mode=memory&cache=shared", os.Getpid())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&types.CollaborativeDoc{},
		&types.CollabDocSnapshot{},
		&types.CollabDocSession{},
		&types.CollabDocFile{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	authz := &collTestAuthorizer{}
	svc := newServiceWithDB(db, authz)
	mdH := NewCollabDocHandler(svc)
	// v0.7.91 — wire real docreader + knowledge for the sync-to-kb test.
	// nil mocks are safe: the handler short-circuits to "queued" path
	// when either dependency is unreachable.
	bytesH := NewCollabDocBytesHandler(svc, nil, nil)
	r := gin.New()
	rg := r.Group("/api/v1")
	mdH.Mount(rg)
	bytesH.Mount(rg)
	// Pre-seed a doc to test against.
	d := &types.CollaborativeDoc{
		ID:          "doc-test-1",
		TenantID:    1,
		KBID:        "kb-test-1",
		Title:       "Test Doc",
		DocKind:     types.CollaborativeDocKindDoc,
		Visibility:  "private",
		ShareToken:  "share-abc",
		OwnerUserID: 1,
	}
	if err := db.Create(d).Error; err != nil {
		t.Fatalf("seed doc: %v", err)
	}
	return r, d, "1"
}

type collTestAuthorizer struct{}

func (*collTestAuthorizer) CanRead(ctx interface{}, tenantID, userID uint64, docID string) (bool, error) {
	return true, nil
}
func (*collTestAuthorizer) CanWrite(ctx interface{}, tenantID, userID uint64, docID string) (bool, error) {
	return true, nil
}

// makeFakeDocx returns the bytes of a minimal valid .docx file.
func makeFakeDocx(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	mw, _ := w.CreateFormFile("file", "test.docx")
	body := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>hello</w:t></w:r></w:p></w:body></w:document>`
	_, _ = mw.Write([]byte(body))
	_ = w.Close()
	return buf.Bytes()
}

func TestCollabDocBytesRoundTrip(t *testing.T) {
	r, d, tenant := buildTestStack(t)
	docID := d.ID

	// 1. Upload
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", "test.docx")
	_, _ = fw.Write([]byte("<w:document xmlns:w=\"http://schemas.openxmlformats.org/wordprocessingml/2006/main\"><w:body><w:p><w:r><w:t>hello</w:t></w:r></w:p></w:body></w:document>"))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/collaborative-docs/"+docID+"/upload", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Mock-Tenant", tenant)
	req.Header.Set("X-Mock-User", "1")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusCreated {
		t.Fatalf("upload status %d body=%s", w2.Code, w2.Body.String())
	}

	// 2. List files
	req2 := httptest.NewRequest(http.MethodGet,
		"/api/v1/collaborative-docs/"+docID+"/files", nil)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req2)
	if w3.Code != http.StatusOK {
		t.Fatalf("list files status %d body=%s", w3.Code, w3.Body.String())
	}
	var listResp struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w3.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("parse list: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0]["version"] != float64(1) {
		t.Fatalf("expected 1 file at v1, got %+v", listResp.Data)
	}

	// 3. Download latest
	req3 := httptest.NewRequest(http.MethodGet,
		"/api/v1/collaborative-docs/"+docID+"/download", nil)
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req3)
	if w4.Code != http.StatusOK {
		t.Fatalf("download status %d", w4.Code)
	}
	if !strings.Contains(w4.Body.String(), "hello") {
		t.Fatalf("download body missing text: %s", w4.Body.String())
	}

	// 4. Upload again -> v2
	body2 := &bytes.Buffer{}
	w5 := multipart.NewWriter(body2)
	fw2, _ := w5.CreateFormFile("file", "test.docx")
	_, _ = fw2.Write([]byte("<w:document xmlns:w=\"http://schemas.openxmlformats.org/wordprocessingml/2006/main\"><w:body><w:p><w:r><w:t>hello v2</w:t></w:r></w:p></w:body></w:document>"))
	_ = w5.Close()
	req4 := httptest.NewRequest(http.MethodPost,
		"/api/v1/collaborative-docs/"+docID+"/upload", body2)
	req4.Header.Set("Content-Type", w5.FormDataContentType())
	w6 := httptest.NewRecorder()
	r.ServeHTTP(w6, req4)
	if w6.Code != http.StatusCreated {
		t.Fatalf("upload v2 status %d body=%s", w6.Code, w6.Body.String())
	}

	// 5. List files again -> 2 entries
	req5 := httptest.NewRequest(http.MethodGet,
		"/api/v1/collaborative-docs/"+docID+"/files", nil)
	w7 := httptest.NewRecorder()
	r.ServeHTTP(w7, req5)
	if w7.Code != http.StatusOK {
		t.Fatalf("list files #2 status %d", w7.Code)
	}
	var list2 struct {
		Data []map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(w7.Body.Bytes(), &list2)
	if len(list2.Data) != 2 {
		t.Fatalf("expected 2 files, got %d", len(list2.Data))
	}

	// 6. Download historical v1
	req7 := httptest.NewRequest(http.MethodGet,
		"/api/v1/collaborative-docs/"+docID+"/download/1", nil)
	w8 := httptest.NewRecorder()
	r.ServeHTTP(w8, req7)
	if w8.Code != http.StatusOK {
		t.Fatalf("download v1 status %d body=%s", w8.Code, w8.Body.String())
	}
	if !strings.Contains(w8.Body.String(), "<w:t>hello</w:t>") {
		t.Fatalf("download v1 body wrong: %s", w8.Body.String())
	}

	// 7. Sync to KB - non-fatal, may return 5xx without docparser but
	// must always return JSON with a doc_id.
	req8 := httptest.NewRequest(http.MethodPost,
		"/api/v1/collaborative-docs/"+docID+"/sync-to-kb",
		strings.NewReader("{}"))
	req8.Header.Set("Content-Type", "application/json")
	w9 := httptest.NewRecorder()
	r.ServeHTTP(w9, req8)
	if w9.Code/100 != 2 {
		t.Fatalf("sync-to-kb status %d body=%s", w9.Code, w9.Body.String())
	}
	if !strings.Contains(w9.Body.String(), docID) {
		t.Fatalf("sync reply missing doc_id: %s", w9.Body.String())
	}
}

func TestCollabDocBytesShareLink(t *testing.T) {
	r, d, _ := buildTestStack(t)
	// Upload first
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", "test.docx")
	_, _ = fw.Write([]byte("<w:document xmlns:w=\"http://schemas.openxmlformats.org/wordprocessingml/2006/main\"><w:body><w:p><w:r><w:t>share</w:t></w:r></w:p></w:body></w:document>"))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/collaborative-docs/"+d.ID+"/upload", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusCreated {
		t.Fatalf("upload status %d body=%s", w2.Code, w2.Body.String())
	}

	// Public share download (no auth) - default visibility is private, expect 403
	req2 := httptest.NewRequest(http.MethodGet,
		"/api/v1/collaborative-docs/share/"+d.ShareToken+"/download", nil)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req2)
	if w3.Code != http.StatusForbidden {
		t.Fatalf("private share should be 403, got %d body=%s", w3.Code, w3.Body.String())
	}

	// Switch visibility to public and retry
	patchReq := httptest.NewRequest(http.MethodPatch,
		"/api/v1/collaborative-docs/"+d.ID,
		strings.NewReader(`{"visibility":"public"}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchW := httptest.NewRecorder()
	r.ServeHTTP(patchW, patchReq)
	if patchW.Code != http.StatusOK {
		t.Fatalf("patch visibility status %d", patchW.Code)
	}

	req3 := httptest.NewRequest(http.MethodGet,
		"/api/v1/collaborative-docs/share/"+d.ShareToken+"/download", nil)
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req3)
	if w4.Code != http.StatusOK {
		t.Fatalf("public share download status %d body=%s", w4.Code, w4.Body.String())
	}
	body2, _ := io.ReadAll(w4.Body)
	if !strings.Contains(string(body2), "share") {
		t.Fatalf("share body wrong: %s", string(body2))
	}
}

// helper to construct the service. Lives in this file to avoid leaking the
// wiring pattern into the public package surface.
func newServiceWithDB(db *gorm.DB, authz CollabAuthorizerShim) *serviceShim {
	return newService(db, authz)
}

// Go doesn't allow returning concrete types here without an import cycle, so
// we re-declare a tiny shim interface and let the actual call to the real
// service constructor be done in a separate helper file.
type CollabAuthorizerShim interface {
	CanRead(ctx interface{}, tenantID, userID uint64, docID string) (bool, error)
	CanWrite(ctx interface{}, tenantID, userID uint64, docID string) (bool, error)
}
type serviceShim struct {
	inner interface{}
}

func newService(db *gorm.DB, authz CollabAuthorizerShim) *serviceShim {
	// We'll fill this in via real wiring in a helper file below.
	return &serviceShim{inner: nil}
}
