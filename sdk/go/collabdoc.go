package weknora

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"

)

// CollabDocService exposes CRUD + binary upload/download for collab docs.
type CollabDocService struct{ c *Client }

// NewCollabDocService constructs a CollabDocService.
func NewCollabDocService(c *Client) *CollabDocService {
	return &CollabDocService{c: c}
}

// Create inserts a new collab doc.
func (s *CollabDocService) Create(ctx context.Context, in  CollabDocInput) (* CollabDoc, error) {
	var out  CollabDoc
	if err := s.c.Do(ctx, "POST", "/collaborative-docs", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns all collab docs the caller can see.
func (s *CollabDocService) List(ctx context.Context) ([] CollabDoc, error) {
	var out [] CollabDoc
	if err := s.c.Do(ctx, "GET", "/collaborative-docs", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UploadBytes stores a new binary version of a collab doc. The SHA256 of
// the bytes is computed by the server and returned in CollabDocFile.
func (s *CollabDocService) UploadBytes(ctx context.Context, docID string, contentType string, data []byte) (* CollabDocFile, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "blob")
	if err != nil {
		return nil, fmt.Errorf("weknora: build multipart: %w", err)
	}
	if _, err := fw.Write(data); err != nil {
		return nil, fmt.Errorf("weknora: write multipart: %w", err)
	}
	w.Close()
	req, err := s.c.NewStreamRequest(ctx, "POST", "/collaborative-docs/"+docID+"/upload", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Body = io.NopCloser(&buf)
	req.ContentLength = int64(buf.Len())
	var out  CollabDocFile
	if err := s.c.DoRaw(ctx, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DownloadBytes fetches the latest binary version of a collab doc.
func (s *CollabDocService) DownloadBytes(ctx context.Context, docID string) ([]byte, error) {
	req, err := s.c.NewStreamRequest(ctx, "GET", "/collaborative-docs/"+docID+"/download", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.c.HTTP().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("weknora: download failed: status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
