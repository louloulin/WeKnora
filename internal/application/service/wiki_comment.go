package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// WikiPageCommentService is the business logic layer for wiki page
// comments. The handler stays thin and delegates here so the same
// validation rules can be reused by future entry points (CLI, MCP tool,
// batch backfill). It mirrors the layering used by wiki_template.go
// and wiki_acl.go in this package.
type WikiPageCommentService struct {
	repo interfaces.WikiCommentRepository
	// pageLookup is a small interface so tests can stub the page
	// existence check without spinning up the full wiki service.
	pageLookup WikiPageExistenceLookup
}

// WikiPageExistenceLookup abstracts the page existence check so the
// comment service can validate "the page exists and is in this KB"
// without depending on the full WikiPageService.
type WikiPageExistenceLookup interface {
	PageExists(ctx context.Context, kbID, pageID string) (bool, error)
}

// NewWikiPageCommentService wires the service.
func NewWikiPageCommentService(repo interfaces.WikiCommentRepository, pageLookup WikiPageExistenceLookup) *WikiPageCommentService {
	return &WikiPageCommentService{repo: repo, pageLookup: pageLookup}
}

// ErrCommentNotFound surfaces the not-found case to the handler so it
// can map to HTTP 404. We deliberately wrap the repository sentinel so
// callers don't need to import the repository package.
var ErrCommentNotFound = errors.New("comment not found")

// ErrCommentForbidden is returned when the actor isn't allowed to edit
// or delete the comment (i.e. not the author and not a KB admin).
var ErrCommentForbidden = errors.New("comment edit/delete forbidden")

// Create validates input, mints an ID + timestamps, and persists.
// authorID comes from the auth context (handler resolves from JWT /
// session); tenantID comes from the auth context too. KB / page IDs
// come from the URL params. Mentions are normalized so empty input
// never serializes as `null` on the wire.
func (s *WikiPageCommentService) Create(
	ctx context.Context,
	kbID, pageID, authorID string,
	tenantID uint64,
	req *types.CreateWikiCommentRequest,
) (*types.WikiPageComment, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, errors.New("body is required")
	}
	if len(body) > 10000 {
		return nil, errors.New("body too long (max 10000 chars)")
	}
	if req.ParentCommentID != nil && *req.ParentCommentID == "" {
		return nil, errors.New("parent_comment_id must not be empty when set")
	}
	// Validate the page exists in this KB before we accept a comment;
	// prevents orphan comments if a stale pageID slips in via the URL.
	if s.pageLookup != nil {
		ok, err := s.pageLookup.PageExists(ctx, kbID, pageID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errors.New("wiki page not found in this knowledge base")
		}
	}
	now := time.Now().UTC()
	c := &types.WikiPageComment{
		ID:              uuid.NewString(),
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
		WikiPageID:      pageID,
		ParentCommentID: req.ParentCommentID,
		AuthorID:        authorID,
		Body:            body,
		Mentions:        normalizeMentions(req.Mentions),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repo.Create(ctx, c); err != nil {
		logger.Errorf(ctx, "wiki comment create failed: %v", err)
		return nil, err
	}
	logger.Infof(ctx, "wiki comment created id=%s page=%s author=%s", c.ID, pageID, authorID)
	return c, nil
}

// ListByPage returns the visible comment thread for a page.
func (s *WikiPageCommentService) ListByPage(
	ctx context.Context, pageID string, limit, offset int,
) ([]*types.WikiPageComment, int64, error) {
	return s.repo.ListByPage(ctx, pageID, limit, offset)
}

// Update is the edit path; only the author (caller-resolved) may edit.
func (s *WikiPageCommentService) Update(
	ctx context.Context, commentID, actorID string, isAdmin bool, body string,
) (*types.WikiPageComment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("body is required")
	}
	if len(body) > 10000 {
		return nil, errors.New("body too long (max 10000 chars)")
	}
	existing, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, repository.ErrWikiPageCommentNotFound) {
			return nil, ErrCommentNotFound
		}
		return nil, err
	}
	if existing.AuthorID != actorID && !isAdmin {
		return nil, ErrCommentForbidden
	}
	existing.Body = body
	existing.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// SetResolved toggles the resolved state. Only KB admins / page authors
// can resolve; we let the handler decide policy and just persist.
func (s *WikiPageCommentService) SetResolved(
	ctx context.Context, commentID, actorID string, isAdmin bool, resolved bool,
) (*types.WikiPageComment, error) {
	existing, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, repository.ErrWikiPageCommentNotFound) {
			return nil, ErrCommentNotFound
		}
		return nil, err
	}
	if existing.AuthorID != actorID && !isAdmin {
		return nil, ErrCommentForbidden
	}
	if err := s.repo.SetResolved(ctx, commentID, resolved, actorID); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, commentID)
}

// Delete soft-deletes the comment. Same authz rules as Update.
func (s *WikiPageCommentService) Delete(ctx context.Context, commentID, actorID string, isAdmin bool) error {
	existing, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, repository.ErrWikiPageCommentNotFound) {
			return ErrCommentNotFound
		}
		return err
	}
	if existing.AuthorID != actorID && !isAdmin {
		return ErrCommentForbidden
	}
	return s.repo.SoftDelete(ctx, commentID)
}

// normalizeMentions guarantees the persisted array is never nil.
// `null` on the wire breaks the existing front-end chip renderer, which
// assumes an array.
func normalizeMentions(in types.StringArray) types.StringArray {
	if in == nil {
		return types.StringArray{}
	}
	return in
}
