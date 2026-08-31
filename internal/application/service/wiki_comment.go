package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// NewWikiCommentService returns a service backed by the supplied repo.
// The service is the single source of truth for tenant scoping,
// validation, and access policy for wiki page comments.
func NewWikiCommentService(repo interfaces.WikiCommentRepository) interfaces.WikiCommentService {
	return &wikiCommentService{repo: repo}
}

type wikiCommentService struct {
	repo interfaces.WikiCommentRepository
}

// Create validates the request, generates a UUID, and persists the row.
// Reply semantics: parent_id may be omitted for a top-level thread, or
// point to an existing comment in the same KB+slug to nest a reply.
func (s *wikiCommentService) Create(
	ctx context.Context,
	tenantID uint64,
	kbID string,
	slug string,
	authorID string,
	authorName string,
	authorAvatar string,
	req types.WikiCommentCreateRequest,
) (*types.WikiComment, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, interfaces.ErrWikiCommentBadInput
	}
	if len(body) > types.WikiCommentMaxBodyBytes {
		return nil, interfaces.ErrWikiCommentBadInput
	}
	if authorID == "" || authorName == "" {
		return nil, interfaces.ErrWikiCommentBadInput
	}
	if kbID == "" || slug == "" {
		return nil, interfaces.ErrWikiCommentBadInput
	}

	mentionsJSON, err := marshalMentions(req.Mentions)
	if err != nil {
		return nil, interfaces.ErrWikiCommentBadInput
	}

	c := &types.WikiComment{
		ID:              uuid.NewString(),
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
		PageSlug:        slug,
		ParentID:        strings.TrimSpace(req.ParentID),
		Body:            body,
		Mentions:        mentionsJSON,
		AnchorBlockID:   strings.TrimSpace(req.AnchorBlockID),
		AuthorID:        authorID,
		AuthorName:      authorName,
		AuthorAvatarURL: authorAvatar,
		CreatedAt:       time.Now(),
	}

	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}

	// If this is a reply, log it for observability.
	if c.ParentID != "" {
		logger.Infof(ctx, "wiki comment reply created: id=%s parent=%s kb=%s", c.ID, c.ParentID, kbID)
	}
	return c, nil
}

// List returns the flattened thread + stats panel for a page.
func (s *wikiCommentService) List(
	ctx context.Context,
	tenantID uint64,
	kbID string,
	slug string,
) (*types.WikiCommentListResponse, error) {
	if kbID == "" || slug == "" {
		return nil, interfaces.ErrWikiCommentBadInput
	}
	rows, err := s.repo.ListByPage(ctx, kbID, slug)
	if err != nil {
		return nil, err
	}
	open, resolved, replies, err := s.repo.CountByPage(ctx, kbID, slug)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []types.WikiComment{}
	}
	return &types.WikiCommentListResponse{
		Comments: rows,
		Stats: types.WikiCommentListStats{
			TotalOpen:     open,
			TotalResolved: resolved,
			TotalReplies:  replies,
		},
	}, nil
}

// Update applies a body + mentions patch. Only the original author may
// edit; resolved threads remain editable so users can refine answers.
func (s *wikiCommentService) Update(
	ctx context.Context,
	tenantID uint64,
	kbID string,
	commentID string,
	authorID string,
	req types.WikiCommentUpdateRequest,
) (*types.WikiComment, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" || len(body) > types.WikiCommentMaxBodyBytes {
		return nil, interfaces.ErrWikiCommentBadInput
	}
	existing, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, interfaces.ErrWikiCommentNotFound
	}
	if existing.KnowledgeBaseID != kbID {
		return nil, interfaces.ErrWikiCommentNotFound
	}
	if existing.AuthorID != authorID {
		return nil, interfaces.ErrWikiCommentForbidden
	}
	mentionsJSON, err := marshalMentions(req.Mentions)
	if err != nil {
		return nil, interfaces.ErrWikiCommentBadInput
	}
	return s.repo.Update(ctx, commentID, body, mentionsJSON)
}

// SetResolved toggles the resolve flag. KB members can resolve threads
// they authored; KB owners / tenant admins can resolve any thread.
func (s *wikiCommentService) SetResolved(
	ctx context.Context,
	tenantID uint64,
	kbID string,
	commentID string,
	actorID string,
	isOwnerOrAdmin bool,
	resolved bool,
) (*types.WikiComment, error) {
	existing, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, interfaces.ErrWikiCommentNotFound
	}
	if existing.KnowledgeBaseID != kbID {
		return nil, interfaces.ErrWikiCommentNotFound
	}
	if !isOwnerOrAdmin && existing.AuthorID != actorID {
		return nil, interfaces.ErrWikiCommentForbidden
	}
	resolvedBy := ""
	if resolved {
		resolvedBy = actorID
	}
	return s.repo.SetResolved(ctx, commentID, resolved, resolvedBy)
}

// Delete removes a comment thread. Only the comment author, KB owner,
// or tenant admin may delete.
func (s *wikiCommentService) Delete(
	ctx context.Context,
	tenantID uint64,
	kbID string,
	commentID string,
	actorID string,
	isOwnerOrAdmin bool,
) error {
	existing, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		return err
	}
	if existing == nil {
		return interfaces.ErrWikiCommentNotFound
	}
	if existing.KnowledgeBaseID != kbID {
		return interfaces.ErrWikiCommentNotFound
	}
	if !isOwnerOrAdmin && existing.AuthorID != actorID {
		return interfaces.ErrWikiCommentForbidden
	}
	return s.repo.Delete(ctx, commentID)
}

// marshalMentions normalises the mention slice into the JSON string
// stored on the comment row.
func marshalMentions(m []types.WikiCommentMention) (string, error) {
	if m == nil {
		m = []types.WikiCommentMention{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Compile-time assertion.
var _ interfaces.WikiCommentService = (*wikiCommentService)(nil)
