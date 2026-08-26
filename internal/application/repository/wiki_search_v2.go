// Package repository — Build #19 / P2.x.a wiki search v2 repository.
//
// Implements PostgreSQL tsvector full-text search with ts_headline for
// snippet generation and ts_rank for ordering. Uses the GIN index from
// migration 000037_wiki_and_indexing.up.sql:73-74 directly via
// `to_tsvector('simple', ...)`.
//
// ACL filtering is deliberately NOT done in SQL — the existing
// WikiAclService.Resolve path covers per-page visibility including group
// expansion, and its in-process cache makes the per-hit Resolve cheap
// (one map lookup + occasional PageOwnerAndAdmin SQL). The search v2
// service layer does the post-filter on the returned hits.
//
// Tenant + visible-KB scoping happens in SQL so cross-tenant data can
// never leak through a misconfigured call site.
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/louloulin/WeKnora/internal/types"
	"gorm.io/gorm"
)

// wikiSearchV2Repository is the Build #19 implementation. The legacy
// `wikiPageRepository.Search` (regex `~*`) is left untouched.
type wikiSearchV2Repository struct {
	db *gorm.DB
}

// NewWikiSearchV2Repository wires a new instance.
func NewWikiSearchV2Repository(db *gorm.DB) WikiSearchV2Repository {
	return &wikiSearchV2Repository{db: db}
}

// Search runs the tsvector query and returns hits.
//
// visibleKBIDs is the KB-ACL pre-filter (KB-level KBAccessRead
// decisions already made by the router). empty / nil means "all KBs
// for this tenant". pageTypes is optional; empty means "any page
// type except archived".
func (r *wikiSearchV2Repository) Search(
	ctx context.Context,
	tenantID uint64,
	_ []string, // denyUserIDs is reserved for future per-user DENY semantics
	req types.WikiSearchV2Request,
	visibleKBIDs []string,
) (types.WikiSearchV2Result, error) {
	start := time.Now()

	result := types.WikiSearchV2Result{
		Hits:   []types.WikiSearchV2Hit{},
		KBIDs:  req.KBIDs,
		Query:  req.Query,
		TookMS: 0,
	}

	if req.Query == "" {
		result.TookMS = int(time.Since(start).Milliseconds())
		return result, nil
	}

	// Pre-compute WHERE fragments with parameterized placeholders. We
	// never inline user input.
	conds := []string{
		"wp.status != ?",
		"wp.tenant_id = ?",
		// to_tsvector over (title weight A + content weight B) using
		// the existing GIN index from migration 000037.
		`(setweight(to_tsvector('simple', coalesce(wp.title, '')), 'A') ||
		  setweight(to_tsvector('simple', coalesce(wp.content, '')), 'B'))
		 @@ plainto_tsquery('simple', ?)`,
	}
	args := []interface{}{types.WikiPageStatusArchived, tenantID, req.Query}

	if len(req.KBIDs) > 0 {
		conds = append(conds, "wp.knowledge_base_id IN ?")
		args = append(args, req.KBIDs)
	} else if len(visibleKBIDs) > 0 {
		conds = append(conds, "wp.knowledge_base_id IN ?")
		args = append(args, visibleKBIDs)
	}

	if len(req.PageTypes) > 0 {
		conds = append(conds, "wp.page_type IN ?")
		args = append(args, req.PageTypes)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	type row struct {
		Slug      string
		Title     string
		PageType  string
		KBID      string
		KBName    string
		Snippet   string
		Rank      float64
		UpdatedAt time.Time
	}

	var rows []row
	q2 := r.db.WithContext(ctx).
		Table("wiki_pages AS wp").
		Select(`
			wp.slug AS slug,
			wp.title AS title,
			wp.page_type AS page_type,
			wp.knowledge_base_id AS kb_id,
			kb.name AS kb_name,
			ts_headline(
				'simple',
				coalesce(wp.title, '') || ' ' || coalesce(wp.content, ''),
				plainto_tsquery('simple', ?),
				'StartSel=<mark>,StopSel=</mark>,MaxFragments=2,MaxWords=20,MinWords=5'
			) AS snippet,
			ts_rank(
				setweight(to_tsvector('simple', coalesce(wp.title, '')), 'A') ||
				setweight(to_tsvector('simple', coalesce(wp.content, '')), 'B'),
				plainto_tsquery('simple', ?)
			) AS rank,
			wp.updated_at AS updated_at
		`, req.Query, req.Query).
		Joins("JOIN knowledge_bases AS kb ON kb.id = wp.knowledge_base_id").
		Where(strings.Join(conds, " AND "), args...).
		Order("rank DESC, wp.updated_at DESC").
		Limit(limit).
		Offset(offset)

	if err := q2.Scan(&rows).Error; err != nil {
		return types.WikiSearchV2Result{}, fmt.Errorf("wiki search v2 query: %w", err)
	}

	hits := make([]types.WikiSearchV2Hit, 0, len(rows))
	for _, row := range rows {
		hits = append(hits, types.WikiSearchV2Hit{
			Slug:      row.Slug,
			Title:     row.Title,
			Snippet:   row.Snippet,
			Score:     row.Rank,
			KBID:      row.KBID,
			KBName:    row.KBName,
			PageType:  row.PageType,
			UpdatedAt: row.UpdatedAt,
		})
	}
	result.Hits = hits
	result.Total = len(hits) // raw count under pagination — total_count requires a separate COUNT(*) query, deferred to Build #19.x
	result.TookMS = int(time.Since(start).Milliseconds())
	return result, nil
}