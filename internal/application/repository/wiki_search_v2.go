// Package repository — Build #19 / P2.x.a wiki search v2 repository.
//
// Implements a three-layer OR tsvector / pg_trgm / LIKE fallback:
//   - `ts_zh`    — Chinese tsvector over jieba-tokenized `content_ts_zh`
//                  (Build #19.x, migration 000096). Only consulted when
//                  the jieba-tokenized query is non-empty.
//   - `ts_simple`— Build #19 default `to_tsvector('simple', ...)` path.
//                  English / Western / digits / identifiers.
//   - `trgm`     — pg_trgm `similarity()` against `lower(title)`. English
//                  typos. Only consulted when req.Fuzzy is true.
//   - `partial`  — `lower(title) LIKE '%q%'`. Last-resort substring
//                  fallback. Only consulted when req.PartialMatch is true.
//
// `matched_by` is computed by a short-circuit CASE expression so a page
// that satisfies multiple arms reports only the highest-priority one
// (`ts_zh > ts_simple > trgm > partial`).
//
// ACL filtering is deliberately NOT done in SQL — the existing
// WikiAclService.Resolve path covers per-page visibility including group
// expansion, and its in-process cache makes the per-hit Resolve cheap.
// The search v2 service layer does the post-filter on the returned hits.
//
// Tenant + visible-KB scoping happens in SQL so cross-tenant data can
// never leak through a misconfigured call site.
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// wikiSearchV2Repository is the Build #19 / #19.x implementation. The
// legacy `wikiPageRepository.Search` (regex `~*`) is left untouched.
type wikiSearchV2Repository struct {
	db *gorm.DB
}

// NewWikiSearchV2Repository wires a new instance.
func NewWikiSearchV2Repository(db *gorm.DB) WikiSearchV2Repository {
	return &wikiSearchV2Repository{db: db}
}

// Search runs the three-layer OR query and returns hits.
//
// `zhQuery` is the jieba-tokenized form of `req.Query` (space-joined
// tokens). Empty when jieba produces nothing — the repo skips the ts_zh
// arm in that case. The handler layer is responsible for tokenizing
// before calling the repo so the SQL stays dialect-agnostic w.r.t. jieba.
func (r *wikiSearchV2Repository) Search(
	ctx context.Context,
	tenantID uint64,
	_ []string, // denyUserIDs is reserved for future per-user DENY semantics
	req types.WikiSearchV2Request,
	visibleKBIDs []string,
	zhQuery string,
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

	// `ts_simple` tsquery — used for the Build #19 arm AND for the
	// `ts_headline` snippet (which concatenates with `||` against `ts_zh`
	// so highlighted tokens appear regardless of which arm matched).
	simpleQuery := req.Query

	// Pre-compute WHERE fragments with parameterized placeholders. We
	// never inline user input.
	conds := []string{
		"wp.status != ?",
		"wp.tenant_id = ?",
	}
	args := []interface{}{types.WikiPageStatusArchived, tenantID}

	// The three-layer OR — short-circuit evaluated in the matching CASE
	// below. Each arm is independently guarded so the planner can skip
	// the `@@` / `similarity()` / `LIKE` work when the arm is disabled
	// (no zh tokens / fuzzy off / partial off).
	matchArms := []string{}
	if zhQuery != "" {
		matchArms = append(matchArms,
			"to_tsvector('simple', coalesce(wp.content_ts_zh, '')) @@ plainto_tsquery('simple', ?)")
	}
	matchArms = append(matchArms,
		`(setweight(to_tsvector('simple', coalesce(wp.title, '')), 'A') ||
		  setweight(to_tsvector('simple', coalesce(wp.content, '')), 'B'))
		 @@ plainto_tsquery('simple', ?)`)
	if req.Fuzzy {
		matchArms = append(matchArms,
			"similarity(lower(coalesce(wp.title, '')), lower(?)) > 0.3")
	}
	if req.PartialMatch {
		matchArms = append(matchArms,
			"lower(coalesce(wp.title, '')) LIKE '%' || lower(?) || '%'")
	}
	conds = append(conds, "("+strings.Join(matchArms, " OR ")+")")

	// Push the OR-bind args in declaration order so the `$N` placeholders
	// line up with the position of the ? in `matchArms`.
	if zhQuery != "" {
		args = append(args, zhQuery)
	}
	args = append(args, simpleQuery)
	if req.Fuzzy {
		args = append(args, req.Query)
	}
	if req.PartialMatch {
		args = append(args, req.Query)
	}

	// KB scoping: explicit request wins, otherwise fall back to the
	// visible-KB set computed by the service layer (KB-ACL intersection).
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

	// Build the snippet tsquery: union of zh and simple so highlighted
	// tokens surface regardless of which arm matched. Skip zh when empty.
	headlineQuery := "plainto_tsquery('simple', ?)"
	headlineArgs := []interface{}{simpleQuery}
	if zhQuery != "" {
		headlineQuery = "(plainto_tsquery('simple', ?) || plainto_tsquery('simple', ?))"
		headlineArgs = []interface{}{zhQuery, simpleQuery}
	}

	// matched_by: short-circuit CASE so the highest-priority arm wins.
	matchedByExpr := `
		CASE
		  WHEN ` + boolExpr(zhQuery != "",
		`to_tsvector('simple', coalesce(wp.content_ts_zh, '')) @@ plainto_tsquery('simple', ?)`,
	) + ` THEN '` + string(types.WikiSearchV2MatchTSZh) + `'
		  WHEN (setweight(to_tsvector('simple', coalesce(wp.title, '')), 'A') ||
		        setweight(to_tsvector('simple', coalesce(wp.content, '')), 'B'))
		       @@ plainto_tsquery('simple', ?) THEN '` + string(types.WikiSearchV2MatchTSSimple) + `'
		  WHEN ` + boolExpr(req.Fuzzy,
		`similarity(lower(coalesce(wp.title, '')), lower(?)) > 0.3`,
	) + ` THEN '` + string(types.WikiSearchV2MatchTrgm) + `'
		  WHEN ` + boolExpr(req.PartialMatch,
		`lower(coalesce(wp.title, '')) LIKE '%' || lower(?) || '%'`,
	) + ` THEN '` + string(types.WikiSearchV2MatchPartial) + `'
		  ELSE 'none'
		END`

	// Args bound for matched_by: order mirrors the WHEN clauses. Note that
	// the simple-query `?` in the second WHEN appears twice (once in the
	// headline tsquery above and once here).
	matchedByArgs := []interface{}{}
	if zhQuery != "" {
		matchedByArgs = append(matchedByArgs, zhQuery)
	}
	matchedByArgs = append(matchedByArgs, simpleQuery)
	if req.Fuzzy {
		matchedByArgs = append(matchedByArgs, req.Query)
	}
	if req.PartialMatch {
		matchedByArgs = append(matchedByArgs, req.Query)
	}

	// Composite rank — sum of the three numeric arms; the simple arm
	// dominates with title weighting because it's the most-tested path.
	rankExpr := `
		COALESCE(ts_rank(
		  setweight(to_tsvector('simple', coalesce(wp.title, '')), 'A') ||
		  setweight(to_tsvector('simple', coalesce(wp.content, '')), 'B'),
		  plainto_tsquery('simple', ?)
		), 0)`

	// SELECT — extends Build #19 with `matched_by`.
	type row struct {
		Slug      string
		Title     string
		PageType  string
		KBID      string
		KBName    string
		Snippet   string
		Rank      float64
		MatchedBy string
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
				`+headlineQuery+`,
				'StartSel=<mark>,StopSel=</mark>,MaxFragments=2,MaxWords=20,MinWords=5'
			) AS snippet,
			`+rankExpr+` AS rank,
			`+matchedByExpr+` AS matched_by,
			wp.updated_at AS updated_at
		`, append(append(append([]interface{}{}, headlineArgs...), simpleQuery), matchedByArgs...)...).
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
			MatchedBy: types.WikiSearchV2MatchSource(row.MatchedBy),
			UpdatedAt: row.UpdatedAt,
		})
	}
	result.Hits = hits
	result.Total = len(hits)
	result.TookMS = int(time.Since(start).Milliseconds())
	return result, nil
}

// boolExpr returns `expr` when `cond` is true, otherwise a literal `FALSE`.
// Keeps the CASE WHEN arms aligned to the same shape whether the optional
// arm is enabled or not.
func boolExpr(cond bool, expr string) string {
	if cond {
		return expr
	}
	return "FALSE"
}
