package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// WikiCategoryMaxDepth is the hard cap on how many folder levels a wiki page's
// category_path may keep. The ingest prompts intentionally ask the model for at
// most 2 levels to keep the directory tree shallow; this storage cap is one
// level deeper as a defensive bound so a slightly over-eager model (or a
// reconcile remap) cannot create an unbounded breadcrumb. It is the single
// source of truth shared by the service, repository, and taxonomy layers so
// stored paths and queried paths are always cleaned identically.
const WikiCategoryMaxDepth = 3

var wikiCategorySeparatorReplacer = strings.NewReplacer("／", "/", "｜", "/", "|", "/")

// CleanWikiCategoryPart normalizes a single raw category label that may itself
// carry embedded separators, wrapping quotes/brackets, or page-type noise, and
// returns the cleaned sub-labels (type labels such as "entity"/"实体" dropped).
func CleanWikiCategoryPart(part string) []string {
	part = strings.TrimSpace(part)
	if part == "" {
		return nil
	}
	part = wikiCategorySeparatorReplacer.Replace(part)
	rawParts := strings.Split(part, "/")
	cleaned := make([]string, 0, len(rawParts))
	for _, raw := range rawParts {
		label := strings.TrimSpace(raw)
		label = strings.Trim(label, `"'“”‘’[]（）()`)
		label = strings.TrimSpace(label)
		if label == "" || isWikiTypeCategoryLabel(label) {
			continue
		}
		cleaned = append(cleaned, label)
	}
	return cleaned
}

// CleanWikiCategoryPath cleans, deduplicates, and caps a full category path at
// WikiCategoryMaxDepth. Centralizing this guarantees that the path a page is
// stored with and the path a list/filter query is matched against go through
// the exact same normalization, so directory filters cannot silently drift.
func CleanWikiCategoryPath(parts []string) []string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, label := range CleanWikiCategoryPart(part) {
			if containsWikiString(cleaned, label) {
				continue
			}
			cleaned = append(cleaned, label)
			if len(cleaned) >= WikiCategoryMaxDepth {
				return cleaned
			}
		}
	}
	return cleaned
}

// SplitWikiPageTypes parses a page_type value that may carry several
// comma-separated types (e.g. "entity,concept") into a deduplicated slice,
// dropping blanks. An empty/whitespace-only input yields nil ("no filter").
// Shared by the handler (query parsing) and repository (List filter) so the
// two layers split identically.
func SplitWikiPageTypes(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	return out
}

func isWikiTypeCategoryLabel(label string) bool {
	normalized := strings.ToLower(strings.TrimSpace(label))
	normalized = strings.TrimSuffix(normalized, "s")
	switch normalized {
	case "entity", "实体", "實體", "concept", "概念", "summary", "摘要", "wiki", "页面", "頁面":
		return true
	default:
		return false
	}
}

func containsWikiString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// WikiPageType constants define the types of wiki pages
const (
	// WikiPageTypeSummary represents a document summary page
	WikiPageTypeSummary = "summary"
	// WikiPageTypeEntity represents an entity page (person, organization, place, etc.)
	WikiPageTypeEntity = "entity"
	// WikiPageTypeConcept represents a concept/topic page
	WikiPageTypeConcept = "concept"
	// WikiPageTypeIndex represents the wiki index page (index.md)
	WikiPageTypeIndex = "index"
	// WikiPageTypeSynthesis represents a synthesis/analysis page.
	// NOT auto-created by ingest — Agent creates these via wiki_write_page tool
	// when it generates cross-document analysis, trends, or insights during conversations.
	WikiPageTypeSynthesis = "synthesis"
	// WikiPageTypeComparison represents a comparison page.
	// NOT auto-created by ingest — Agent creates these via wiki_write_page tool
	// when the user asks to compare entities, concepts, or approaches.
	WikiPageTypeComparison = "comparison"
)

// WikiPageStatus constants
const (
	// WikiPageStatusDraft indicates the page is a draft
	WikiPageStatusDraft = "draft"
	// WikiPageStatusPublished indicates the page is published and visible
	WikiPageStatusPublished = "published"
	// WikiPageStatusArchived indicates the page is archived
	WikiPageStatusArchived = "archived"
)

// IsValidWikiPageType reports whether pageType is one of the known page
// types. Unknown values would silently disappear from the type-filtered
// listings the browser is built on, so write paths reject them.
func IsValidWikiPageType(pageType string) bool {
	switch pageType {
	case WikiPageTypeSummary, WikiPageTypeEntity, WikiPageTypeConcept,
		WikiPageTypeIndex, WikiPageTypeSynthesis, WikiPageTypeComparison:
		return true
	default:
		return false
	}
}

// IsValidWikiPageStatus reports whether status is one of the known statuses.
func IsValidWikiPageStatus(status string) bool {
	switch status {
	case WikiPageStatusDraft, WikiPageStatusPublished, WikiPageStatusArchived:
		return true
	default:
		return false
	}
}

// WikiPage represents a single wiki page in a wiki knowledge base.
// Wiki pages are LLM-generated, interlinked markdown documents that form
// a persistent, compounding knowledge artifact.
type WikiPage struct {
	// Unique identifier (UUID)
	ID string `json:"id" gorm:"type:varchar(36);primaryKey"`
	// Workspace ID for multi-workspace isolation
	TenantID uint64 `json:"tenant_id" gorm:"index"`
	// Knowledge base this page belongs to
	KnowledgeBaseID string `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	// URL-friendly slug for addressing, e.g. "entity/acme-corp", "concept/rag"
	// Unique within a knowledge base
	Slug string `json:"slug" gorm:"type:varchar(255);uniqueIndex:idx_kb_slug"`
	// Human-readable title
	Title string `json:"title" gorm:"type:varchar(512)"`
	// Page type: summary, entity, concept, index, synthesis, comparison
	PageType string `json:"page_type" gorm:"type:varchar(32);index"`
	// Page status: draft, published, archived
	Status string `json:"status" gorm:"type:varchar(32);default:'published'"`
	// Full markdown content
	Content string `json:"content" gorm:"type:text"`
	// ContentHTML is an optional HTML projection of Content, written by the
	// WYSIWYG editor (Build #2b, feature-flag gated). NULL on every legacy
	// row and on pages that have never been edited through the rich editor —
	// readers fall back to Content in that case. Not part of revision
	// snapshots; the markdown column remains the authoritative source for
	// versioning and rollback.
	ContentHTML string `json:"content_html,omitempty" gorm:"column:content_html;type:text"`
	// ContentTSZh is the jieba-tokenized projection of (Title + Content),
	// stored as space-joined tokens and indexed via a GIN expression
	// `to_tsvector('simple', content_ts_zh)` (see migration 000096). It backs
	// Build #19.x's `@@ plainto_tsquery('simple', $jieba)` arm of the
	// three-layer OR. NULL is acceptable on rows that pre-date 000096;
	// the server backfill loop fills them in on startup.
	ContentTSZh string `json:"-" gorm:"column:content_ts_zh;type:text"`
	// One-line summary for index listing
	Summary string `json:"summary" gorm:"type:text"`
	// Alternate names, abbreviations, acronyms or translated names
	Aliases StringArray `json:"aliases" gorm:"type:json"`
	// ParentSlug optionally points at the wiki page that should act as this
	// page's semantic parent in the directory tree. The parent may be empty
	// when the page is grouped only by FolderID.
	ParentSlug string `json:"parent_slug,omitempty" gorm:"type:varchar(255);index"`
	// FolderID is the single source of truth for where this page sits in the
	// directory tree — a reference to wiki_folders.id ("" = wiki root). The
	// CategoryPath / WikiPath / Depth fields below are denormalized caches
	// recomputed from this folder's chain on every write so list/index/search
	// queries don't have to join wiki_folders.
	FolderID string `json:"folder_id,omitempty" gorm:"column:folder_id;type:varchar(36);index;default:''"`
	// CategoryPath is the directory breadcrumb that groups this page in the
	// wiki browser, e.g. ["AI", "LLM 应用", "RAG"]. Derived cache of the
	// folder chain identified by FolderID.
	CategoryPath StringArray `json:"category_path,omitempty" gorm:"type:json"`
	// WikiPath is a normalized, sortable path derived from page_type,
	// category_path, and title. It keeps large directory listings cheap to sort.
	WikiPath string `json:"wiki_path,omitempty" gorm:"type:varchar(1024);index"`
	// Depth is len(CategoryPath), cached for filtering / display.
	Depth int `json:"depth,omitempty" gorm:"default:0;index"`
	// SortOrder allows generated or manually edited pages to control sibling
	// ordering before falling back to title.
	SortOrder int `json:"sort_order,omitempty" gorm:"default:0;index"`
	// References to source knowledge IDs that contributed to this page.
	// Format matches the legacy "<knowledge_id>|<doc_title>" convention used
	// across the ingest pipeline, so retract / display code can split on `|`
	// to recover the title. Document-level granularity.
	SourceRefs StringArray `json:"source_refs" gorm:"type:json"`
	// ChunkRefs records the specific source-document chunks this page was
	// built from — one UUID per cited chunk. Populated during ingest from
	// the chunk-citation pass; refreshed wholesale whenever the page is
	// re-materialized. Empty for summary pages (they are document-level
	// synopses and don't carry chunk-level citations). Use this when you
	// need to surface the underlying evidence for a wiki page, or to
	// retract citations when a source document is deleted.
	ChunkRefs StringArray `json:"chunk_refs" gorm:"type:json"`
	// Slugs of pages that link TO this page (backlinks)
	InLinks StringArray `json:"in_links" gorm:"type:json"`
	// Slugs of pages this page links to (outbound links)
	OutLinks StringArray `json:"out_links" gorm:"type:json"`
	// Arbitrary metadata (tags, categories, dates, etc.)
	PageMetadata JSON `json:"page_metadata" gorm:"column:page_metadata;type:json"`
	// Version number. Incremented only when a user-visible content field
	// (title, content, summary, page_type, status) actually changes; pure
	// bookkeeping writes (link maintenance, same-content re-ingest, status
	// sync from background jobs) leave it untouched so it can be used as a
	// real "the page was edited" signal.
	Version int `json:"version" gorm:"default:1"`
	// LastEditSource records who authored the CURRENT version: pipeline |
	// agent | user | revert. Empty for legacy rows (treated as pipeline).
	// When the version is superseded this value travels into the revision
	// snapshot, so each historical version keeps its own author kind.
	LastEditSource string `json:"last_edit_source,omitempty" gorm:"type:varchar(16);default:''"`
	// LastEditorID is the user id of the caller that produced the current
	// version (empty for background pipeline writes).
	LastEditorID string `json:"last_editor_id,omitempty" gorm:"type:varchar(64);default:''"`
	// Creation time
	CreatedAt time.Time `json:"created_at"`
	// Last update time
	UpdatedAt time.Time `json:"updated_at"`
	// Soft delete
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// TableName specifies the database table name
func (WikiPage) TableName() string {
	return "wiki_pages"
}

// Wiki edit source constants describe who authored a page version. Stored in
// WikiPage.LastEditSource (current version) and WikiPageRevision.EditSource
// (historical versions).
const (
	// WikiEditSourcePipeline marks versions written by the wiki ingest
	// pipeline (also the fallback for legacy rows with an empty source).
	WikiEditSourcePipeline = "pipeline"
	// WikiEditSourceAgent marks versions written through the agent wiki
	// tools (wiki_write_page / wiki_replace_text / ...).
	WikiEditSourceAgent = "agent"
	// WikiEditSourceUser marks versions written by a human through the
	// wiki editor UI / REST API.
	WikiEditSourceUser = "user"
	// WikiEditSourceRevert marks versions produced by rolling the page
	// back to an earlier revision.
	WikiEditSourceRevert = "revert"
)

// NormalizeWikiEditSource maps unknown / empty values to
// WikiEditSourcePipeline so legacy rows and forgotten call sites degrade to
// the historical behavior ("the machine wrote this").
func NormalizeWikiEditSource(source string) string {
	switch source {
	case WikiEditSourceAgent, WikiEditSourceUser, WikiEditSourceRevert, WikiEditSourcePipeline:
		return source
	default:
		return WikiEditSourcePipeline
	}
}

// WikiPageRevision is one immutable snapshot of a superseded page version.
// The current version lives only in wiki_pages; when an edit replaces
// version V the pre-edit state is inserted here as (page_id, V) inside the
// same transaction, so every historical version stays diffable and
// revertable. Rows are pruned per WikiRevisionPruneRequest to bound storage
// on hot pipeline pages.
type WikiPageRevision struct {
	ID              string      `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID        uint64      `json:"tenant_id" gorm:"index"`
	KnowledgeBaseID string      `json:"knowledge_base_id" gorm:"type:varchar(36);index:idx_wiki_page_revisions_kb_slug"`
	PageID          string      `json:"page_id" gorm:"type:varchar(36);uniqueIndex:idx_wiki_page_revisions_page_version"`
	Slug            string      `json:"slug" gorm:"type:varchar(255);index:idx_wiki_page_revisions_kb_slug"`
	Version         int         `json:"version" gorm:"uniqueIndex:idx_wiki_page_revisions_page_version"`
	Title           string      `json:"title" gorm:"type:varchar(512)"`
	PageType        string      `json:"page_type" gorm:"type:varchar(32)"`
	Status          string      `json:"status" gorm:"type:varchar(32)"`
	Content         string      `json:"content,omitempty" gorm:"type:text"`
	Summary         string      `json:"summary" gorm:"type:text"`
	Aliases         StringArray `json:"aliases" gorm:"type:json"`
	// Author of THIS version (same semantics as WikiPage.LastEditSource).
	EditSource string `json:"edit_source" gorm:"type:varchar(16);default:''"`
	EditorID   string `json:"editor_id" gorm:"type:varchar(64);default:''"`
	// When this version was authored (the page's updated_at while current).
	EditedAt  time.Time `json:"edited_at"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName specifies the database table name
func (WikiPageRevision) TableName() string {
	return "wiki_page_revisions"
}

// Snapshot retention is two-tiered, because the two kinds of history have
// very different value. A hot hub page is rewritten by the ingest pipeline on
// every related batch, so machine snapshots would otherwise crowd out the
// handful of human/agent edits this feature exists to protect.
const (
	// WikiMaxRevisionsPerPage is how many recent versions are kept for
	// prunable (machine-authored) snapshots.
	WikiMaxRevisionsPerPage = 50
	// WikiMaxRevisionsHardCap is the absolute per-page ceiling, applied
	// regardless of author, so a page edited only by hand still has bounded
	// storage.
	WikiMaxRevisionsHardCap = 200
)

// WikiPrunableEditSources lists the edit sources whose snapshots may be
// dropped by the soft cap. Everything else (user / agent / revert) survives
// until the hard cap. Legacy rows carry an empty source, hence "".
var WikiPrunableEditSources = []string{"", WikiEditSourcePipeline}

// WikiRevisionPruneRequest describes the two-tier retention applied to one
// page's snapshot history. A version threshold <= 0 disables that tier.
type WikiRevisionPruneRequest struct {
	PageID string
	// KeepFromVersion: snapshots below this version are dropped, but only
	// when their edit source is listed in PrunableSources.
	KeepFromVersion int
	PrunableSources []string
	// HardKeepFromVersion: snapshots below this version are always dropped,
	// whatever their author.
	HardKeepFromVersion int
}

// WikiPageUpdateRequest is the partial-update payload for
// PUT /wiki/pages/*slug. All content fields are optional pointers — absent
// fields keep their stored value, so a client can change just the body
// without re-sending (and risking clobbering) title/status/aliases.
type WikiPageUpdateRequest struct {
	Title    *string      `json:"title,omitempty"`
	Content  *string      `json:"content,omitempty"`
	// ContentHTML is the WYSIWYG editor's HTML projection, optional. nil =
	// leave the stored column untouched (so a markdown-only client doesn't
	// accidentally clobber an HTML value the WYSIWYG editor previously wrote).
	ContentHTML *string     `json:"content_html,omitempty"`
	Summary     *string     `json:"summary,omitempty"`
	PageType    *string     `json:"page_type,omitempty"`
	Status      *string     `json:"status,omitempty"`
	Aliases     *StringArray `json:"aliases,omitempty"`
	// Version is the optimistic-lock guard: when > 0 the update is rejected
	// with a conflict if the stored version differs (someone else edited the
	// page since the client loaded it). 0 skips the check (legacy clients).
	Version int `json:"version,omitempty"`
}

// WikiPageRevisionListResponse is the payload for GET /wiki/revisions/*slug.
// Revisions hold the superseded versions (content omitted in list mode);
// the current version is described by CurrentVersion + the page row itself,
// which the frontend already has.
type WikiPageRevisionListResponse struct {
	Revisions      []*WikiPageRevision `json:"revisions"`
	Total          int64               `json:"total"`
	CurrentVersion int                 `json:"current_version"`
}

// WikiPageRevertRequest rolls a page back to one of its stored revisions.
// Slug travels in the body (not the path) for the same reason as
// WikiPageMoveRequest: hierarchical slugs collide with gin's catch-all.
type WikiPageRevertRequest struct {
	Slug    string `json:"slug" binding:"required"`
	Version int    `json:"version" binding:"required"`
}

// WikiFolderRootID is the sentinel parent/folder id meaning "the wiki root"
// (a page or folder directly under the top level, with no parent folder).
const WikiFolderRootID = ""

// WikiFolder is a first-class directory node in the wiki browser. Folders
// exist independently of pages — an empty folder persists so users can lay
// out a skeleton and file pages into it later. The tree is an adjacency list
// (ParentID, "" = root); Path is the materialized "/"-joined name chain kept
// purely for cheap display/sort. A wiki page's placement is WikiPage.FolderID.
type WikiFolder struct {
	ID              string         `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID        uint64         `json:"tenant_id" gorm:"index"`
	KnowledgeBaseID string         `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	ParentID        string         `json:"parent_id" gorm:"column:parent_id;type:varchar(36);index;default:''"`
	Name            string         `json:"name" gorm:"type:varchar(255)"`
	Path            string         `json:"path" gorm:"type:varchar(1024)"`
	Depth           int            `json:"depth" gorm:"default:0"`
	SortOrder       int            `json:"sort_order" gorm:"default:0"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// TableName specifies the database table name
func (WikiFolder) TableName() string {
	return "wiki_folders"
}

// WikiFolderNode is one directory node returned to the browser, enriched with
// the live page count directly under it and whether it has child folders so
// the UI can render an expand affordance without a second round-trip.
type WikiFolderNode struct {
	WikiFolder
	PageCount   int64 `json:"page_count"`
	HasChildren bool  `json:"has_children"`
}

// WikiFolderListResponse is the payload for listing the direct children of a
// folder (parent_id="" = root level).
type WikiFolderListResponse struct {
	ParentID string           `json:"parent_id"`
	Folders  []WikiFolderNode `json:"folders"`
}

// WikiFolderCreateRequest creates a new (initially empty) folder under ParentID.
type WikiFolderCreateRequest struct {
	ParentID string `json:"parent_id"`
	Name     string `json:"name"`
}

// WikiFolderUpdateRequest renames and/or reparents a folder. ParentID is
// applied only when MoveParent is true so a pure rename doesn't have to
// re-send the (possibly root "") parent and risk an accidental move.
type WikiFolderUpdateRequest struct {
	Name       string `json:"name,omitempty"`
	ParentID   string `json:"parent_id,omitempty"`
	MoveParent bool   `json:"move_parent,omitempty"`
}

// WikiPageMoveRequest relocates the page identified by Slug into FolderID
// ("" = root). Slug is carried in the body (not the path) because wiki slugs
// are hierarchical ("entity/acme") and would collide with gin's catch-all.
type WikiPageMoveRequest struct {
	Slug     string `json:"slug" binding:"required"`
	FolderID string `json:"folder_id"`
}

// MaxWikiBatchSize caps how many slugs a single wiki batch endpoint may
// accept. 100 protects DB locks + Redis cache pressure and aligns with
// BatchDeleteKnowledge. Excess requests 400 with too_many.
const MaxWikiBatchSize = 100

// WikiPageBatchMoveRequest relocates up to MaxWikiBatchSize pages into the
// same folder. Per-page failures do not abort the batch (partial-success
// semantics, D1 in brief). Slugs are deduped server-side.
//
// Build #12.
type WikiPageBatchMoveRequest struct {
	Slugs    []string `json:"slugs" binding:"required"`
	FolderID string   `json:"folder_id"`
}

// WikiPageBatchDeleteRequest soft-deletes up to MaxWikiBatchSize pages in
// one transaction. Each page triggers its own removeInLinks cascade against
// every target it referenced, matching DeletePage behaviour.
//
// Build #12.
type WikiPageBatchDeleteRequest struct {
	Slugs []string `json:"slugs" binding:"required"`
}

// WikiPageBatchStatusRequest rewrites `status` for up to MaxWikiBatchSize
// pages in one bookkeeping-only update (no version bump). Status values are
// restricted to draft / published / archived by IsValidWikiPageStatus.
//
// Build #12.
type WikiPageBatchStatusRequest struct {
	Slugs  []string `json:"slugs" binding:"required"`
	Status string   `json:"status" binding:"required"`
}

// WikiPageBatchFailure is one per-page failure entry inside WikiBatchResult.
// Slug is echoed back so the UI can pin the failing row, and Code is a
// stable machine-readable token ("not_found" | "kb_mismatch" | "invalid"
// | "forbidden") so the frontend can render an i18n string per category
// without re-parsing Error.
//
// Build #12.
type WikiPageBatchFailure struct {
	Slug  string `json:"slug"`
	Code  string `json:"code"`
	Error string `json:"error"`
}

// WikiBatchResult is the response shape shared by all three wiki batch
// endpoints. Succeeded holds the slugs that were actually applied (after
// dedup); Failed captures per-row errors so the caller can surface partial
// success to the user.
//
// Build #12.
type WikiBatchResult struct {
	Succeeded []string               `json:"succeeded"`
	Failed    []WikiPageBatchFailure `json:"failed"`
}

// WikiBatchPreviewSummary is the head-count triple the Build #16 preview
// dialog renders next to the per-slug table. It is purely informational —
// the authoritative per-slug outcome lives in WikiBatchPreviewResponse.
type WikiBatchPreviewSummary struct {
	Total       int `json:"total"`
	WillSucceed int `json:"will_succeed"`
	WillFail    int `json:"will_fail"`
}

// WikiBatchPreviewResponse is the dry-run analogue of WikiBatchResult.
// Success holds slugs that would apply without error; Failed mirrors the
// same {Slug, Code, Error} triple WikiBatchResult uses so the frontend can
// reuse the i18n key map (WikiBatchErrorCodeToI18nKey) without a second
// translation namespace.
//
// The preview is computed by reading the row + running only the
// validation rules the matching real call would (folder_id resolve,
// status validity, slug existence) — no writes, no cascades.
//
// Build #16.
type WikiBatchPreviewResponse struct {
	Success []string               `json:"success"`
	Failed  []WikiPageBatchFailure `json:"failed"`
	Summary WikiBatchPreviewSummary `json:"summary"`
}

// WikiBatchAsyncThreshold is the slug count above which the batch endpoints
// enqueue an async job instead of executing synchronously. Below this the
// whole request runs in-process and returns the WikiBatchResult directly;
// above it the request returns a job ID and a worker pool drains it.
//
// Threshold 20 keeps the synchronous path's worst-case latency under ~1s
// on a typical 4-core KB while still letting the UI fire-and-forget large
// archival/cleanup operations.
//
// Build #13.
const WikiBatchAsyncThreshold = 20

// WikiBatchJobType enumerates the four supported batch job categories.
// `tag` is reserved for Build #15 — listed here so the type is stable now
// rather than renamed later (changing the column constraint is a migration).
//
// Build #13.
type WikiBatchJobType string

const (
	WikiBatchJobTypeMove   WikiBatchJobType = "move"
	WikiBatchJobTypeDelete WikiBatchJobType = "delete"
	WikiBatchJobTypeStatus WikiBatchJobType = "status"
	WikiBatchJobTypeTag    WikiBatchJobType = "tag"
)

// WikiBatchJobState is the lifecycle of one queued batch operation.
// queued → running → (succeeded | failed | partial). Workers advance the
// state in this exact order; the frontend polls GET /batch-jobs/:id and
// shows progress + the final result.
//
// Build #13.
type WikiBatchJobState string

const (
	WikiBatchJobStateQueued    WikiBatchJobState = "queued"
	WikiBatchJobStateRunning   WikiBatchJobState = "running"
	WikiBatchJobStateSucceeded WikiBatchJobState = "succeeded"
	WikiBatchJobStateFailed    WikiBatchJobState = "failed"
	WikiBatchJobStatePartial   WikiBatchJobState = "partial"
)

// WikiBatchJob is the persisted shape of one async batch operation.
// Params carries type-specific input (slugs + folder_id / target status);
// UndoState captures per-page original values captured before mutation
// so UndoJob can roll back deterministically without re-fetching.
//
// Build #13.
type WikiBatchJob struct {
	ID               string                 `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID         uint64                 `json:"tenant_id" gorm:"index"`
	KnowledgeBaseID  string                 `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	Type             WikiBatchJobType       `json:"type" gorm:"type:varchar(16)"`
	Params           JSON                   `json:"params" gorm:"type:jsonb"`
	UndoState        JSON                   `json:"undo_state,omitempty" gorm:"type:jsonb"`
	State            WikiBatchJobState      `json:"state" gorm:"type:varchar(16);default:'queued'"`
	Result           JSON                   `json:"result,omitempty" gorm:"type:jsonb"`
	// Progress carries running counters published by the worker on a
	// throttled cadence (every 5 slugs, or on terminal). Shape:
	//   { total, processed, succeeded, failed, updated_at }.
	// Build #15.
	Progress         JSON                   `json:"progress,omitempty" gorm:"type:jsonb"`
	CreatedBy        string                 `json:"created_by" gorm:"type:varchar(64)"`
	CreatedAt        time.Time              `json:"created_at"`
	StartedAt        *time.Time             `json:"started_at,omitempty"`
	FinishedAt       *time.Time             `json:"finished_at,omitempty"`
	ExpiresAt        *time.Time             `json:"expires_at,omitempty"`
}

// TableName points at the migration-managed table.
func (WikiBatchJob) TableName() string {
	return "wiki_batch_jobs"
}

// Undoable reports whether UndoJob can roll this job back. Only `move` and
// `delete` are reversible; `status` would require re-applying the prior
// status which is semantically confusing and not worth the complexity.
// `tag` is reserved for Build #15.
//
// Build #13.
func (j *WikiBatchJob) Undoable() bool {
	switch j.Type {
	case WikiBatchJobTypeMove, WikiBatchJobTypeDelete:
		return true
	}
	return false
}

// Expired reports whether the persistent undo window (7 days after finish)
// has passed. After Expired() UndoJob returns 410 Gone.
//
// Build #13.
func (j *WikiBatchJob) Expired(now time.Time) bool {
	return j.ExpiresAt != nil && now.After(*j.ExpiresAt)
}

// WikiBatchJobParams is the type-discriminated payload carried in
// WikiBatchJob.Params. Only the fields relevant to the job's Type are
// populated; the rest are zero values.
//
// Build #13.
type WikiBatchJobParams struct {
	Slugs    []string `json:"slugs"`
	FolderID string   `json:"folder_id,omitempty"`
	Status   string   `json:"status,omitempty"`
}

// WikiBatchJobUndoState captures the per-page state required to roll a
// job back. For `move` jobs we keep the previous folder_id; for `delete`
// jobs we keep the slug + original folder_id + status (so undo can also
// decide whether to keep the page archived / unpublished after restore).
//
// Build #13.
type WikiBatchJobUndoState struct {
	// PageStates maps slug -> pre-mutation state. Undo iterates this map
	// and applies each entry via the inverse service call.
	PageStates map[string]WikiBatchJobUndoPageState `json:"page_states"`
}

// WikiBatchJobUndoPageState is one row of WikiBatchJobUndoState.PageStates.
// Build #13.
type WikiBatchJobUndoPageState struct {
	FolderID string `json:"folder_id"`
	Status   string `json:"status"`
}

// WikiBatchJobResult is what workers write back to WikiBatchJob.Result.
// Mirrors WikiBatchResult but typed as a JSON column (no GORM struct
// scanning needed). The frontend polls this directly.
//
// Build #13.
type WikiBatchJobResult struct {
	Succeeded []string               `json:"succeeded"`
	Failed    []WikiPageBatchFailure `json:"failed"`
}

// WikiBatchJobProgress is the running-counter snapshot the worker
// publishes into WikiBatchJob.Progress on a throttled cadence (every
// 5 slugs, or on terminal). Total is captured at enqueue time so the
// UI can render a "{processed}/{total}" fraction even after the batch
// has finished. Processed >= Succeeded + Failed at every snapshot.
//
// Build #15.
type WikiBatchJobProgress struct {
	Total     int       `json:"total"`
	Processed int       `json:"processed"`
	Succeeded int       `json:"succeeded"`
	Failed    int       `json:"failed"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WikiBatchJobFailureRecord is one row of wiki_batch_job_failures — the
// per-slug failure ledger. Distinct from WikiBatchJobAuditEvent
// (Build #14) which records "what happened when" — this table records
// "which slug failed because of what", so the UI can group errors by
// code without parsing the result JSONB blob.
//
// Build #15.
type WikiBatchJobFailureRecord struct {
	ID              int64     `json:"id"            gorm:"primaryKey"`
	TenantID        uint64    `json:"tenant_id"     gorm:"index"`
	KnowledgeBaseID string    `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	BatchJobID      string    `json:"batch_job_id"  gorm:"type:varchar(36);index"`
	Slug            string    `json:"slug"`
	Code            string    `json:"code"`
	Error           string    `json:"error"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// TableName points at the migration-managed table.
func (WikiBatchJobFailureRecord) TableName() string {
	return "wiki_batch_job_failures"
}

// WikiBatchFailureGroupCount is the aggregated count for one error
// code returned by the failure drawer's "group by code" view.
//
// Build #15.
type WikiBatchFailureGroupCount struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

// WikiBatchFailureListResponse is the wire shape of
// GET /batch-jobs/:id/failures. Events + total + a parallel slice of
// per-code group counts so the drawer can render its code tabs in a
// single round-trip.
//
// Build #15.
type WikiBatchFailureListResponse struct {
	Failures  []WikiBatchJobFailureRecord   `json:"failures"`
	Groups    []WikiBatchFailureGroupCount   `json:"groups"`
	Total     int                           `json:"total"`
	Page      int                           `json:"page"`
	PageSize  int                           `json:"page_size"`
}

// WikiBatchRouteResult is the discriminated response for the three
// /batch-* endpoints under the auto-router. Kind = "sync" means the
// whole batch ran in-process and Result holds the per-row outcome;
// Kind = "job" means the request was queued for the worker pool and
// Job carries the job id for polling / undo. The handler decides the
// HTTP status (sync → 200 with the result; job → 202 with the job).
//
// Build #13.
type WikiBatchRouteResult struct {
	Kind   string           `json:"kind"`              // "sync" | "job"
	Result *WikiBatchResult `json:"result,omitempty"`
	Job    *WikiBatchJob    `json:"job,omitempty"`
}

// ErrWikiBatchKBMismatch is returned by every batch service method when
// the request contains at least one slug that exists in a different
// knowledge base. Per the brief's D2, this is a request-level rejection
// (HTTP 400 + code `kb_mismatch`), not a per-row partial failure — a
// caller asking to mutate KB-A's bulk endpoint with KB-B's slug is
// almost certainly a stale-client bug, and silently dropping the slug
// would mask the root cause.
//
// Build #12.
var ErrWikiBatchKBMismatch = errors.New("wiki batch: slug belongs to a different knowledge base")

// WikiBatchKBMismatchError wraps ErrWikiBatchKBMismatch with the offending
// slug so the handler can echo it in the 400 body.
type WikiBatchKBMismatchError struct {
	Slug     string
	ActualKB string
}

func (e *WikiBatchKBMismatchError) Error() string {
	return fmt.Sprintf("wiki batch: slug %q belongs to knowledge base %q, not the requested one", e.Slug, e.ActualKB)
}

func (e *WikiBatchKBMismatchError) Unwrap() error { return ErrWikiBatchKBMismatch }

// IsWikiBatchKBMismatch reports whether err originated from the cross-KB
// guard in any of the batch endpoints.
func IsWikiBatchKBMismatch(err error) bool {
	return errors.Is(err, ErrWikiBatchKBMismatch)
}

// WikiBatchJob sentinel errors. The HTTP handler maps each to a stable
// status code (see internal/handler/wiki_page.go). Wrapped where extra
// context is needed; callers should errors.Is on these.
//
// Build #13.
var (
	// ErrWikiBatchJobNotFound — job id does not exist (or belongs to a
	// different KB; the handler returns 404 for both to avoid leaking
	// existence across KBs).
	ErrWikiBatchJobNotFound = errors.New("wiki batch job not found")
	// ErrWikiBatchJobNotUndoable — the job's Type is not reversible
	// (currently only `status` and `tag`). Handler returns 422.
	ErrWikiBatchJobNotUndoable = errors.New("wiki batch job is not undoable")
	// ErrWikiBatchJobExpired — the 7-day undo window has passed.
	// Handler returns 410 Gone.
	ErrWikiBatchJobExpired = errors.New("wiki batch job undo window expired")
	// ErrWikiBatchJobAlreadyDone — terminal state — the job is in
	// succeeded/failed/partial AND a previous undo already ran
	// (we leave a sentinel marker). Handler returns 409.
	ErrWikiBatchJobAlreadyDone = errors.New("wiki batch job already finalized")
	// ErrWikiBatchJobNone — repository returned no queued jobs
	// (ClaimNextQueued internal signal). Internal only; not exposed to
	// the handler.
	ErrWikiBatchJobNone = errors.New("no wiki batch jobs queued")
	// ErrWikiBatchJobNotCancellable — the job has already left the
	// queued state and cannot be aborted. Surfaced as 409 by the
	// handler. Build #14.
	ErrWikiBatchJobNotCancellable = errors.New("wiki batch job is not cancellable")
)

// WikiBatchAuditAction enumerates the event kinds we record in
// wiki_batch_job_audit. The constant set is closed at write-time —
// repository/service code uses these as keys, the SQL CHECK is left
// open (Build #14 D2) so we can add new events without ALTER TABLE.
//
// Build #14.
type WikiBatchAuditAction string

const (
	// WikiBatchAuditActionEnqueue — user submitted a batch request that
	// was queued (async path).
	WikiBatchAuditActionEnqueue WikiBatchAuditAction = "enqueue"
	// WikiBatchAuditActionStart — worker claimed the job and advanced
	// state to running.
	WikiBatchAuditActionStart WikiBatchAuditAction = "start"
	// WikiBatchAuditActionFinish — worker wrote the terminal result
	// (succeeded/failed/partial). Metadata carries the per-action
	// counts and error codes.
	WikiBatchAuditActionFinish WikiBatchAuditAction = "finish"
	// WikiBatchAuditActionUndoRequest — user invoked the undo endpoint.
	// Recorded before undo runs so a half-failed undo is still
	// traceable.
	WikiBatchAuditActionUndoRequest WikiBatchAuditAction = "undo_request"
	// WikiBatchAuditActionUndoDone — undo completed (success or
	// partial). Metadata carries restored_count + skipped_count.
	WikiBatchAuditActionUndoDone WikiBatchAuditAction = "undo_done"
	// WikiBatchAuditActionCancel — EnqueueJob hit a full channel and
	// degraded to synchronous execution. Metadata carries
	// `{reason: "queue_full"}`.
	WikiBatchAuditActionCancel WikiBatchAuditAction = "cancel"
	// WikiBatchAuditActionExpire — periodic cleanup observed an
	// expired job (passed expires_at). Actor is always "system".
	WikiBatchAuditActionExpire WikiBatchAuditAction = "expire"
)

// WikiBatchJobAuditEvent is one immutable audit row. The repository
// inserts and never updates; consumers (handlers, audits page) only
// read.
//
// Build #14.
type WikiBatchJobAuditEvent struct {
	ID              int64                  `json:"id"`
	TenantID        uint64                 `json:"tenant_id"`
	KnowledgeBaseID string                 `json:"knowledge_base_id"`
	BatchJobID      string                 `json:"batch_job_id"`
	Action          WikiBatchAuditAction   `json:"action"`
	ActorID         string                 `json:"actor_id"`
	OccurredAt      time.Time              `json:"occurred_at"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// WikiBatchAuditActorSystem is the actor reserved for events not
// driven by an HTTP request (worker start / finish / undo_done via
// async worker, periodic expire cleanup, queue-full cancel). Always
// lower-case so the audit filter dropdown can match exactly.
//
// Build #14.
const WikiBatchAuditActorSystem = "system"

// IsWikiBatchAuditTerminalAction returns true for events that mark
// the end of a sub-flow and are typically the last row the audit
// drawer shows when expanded.
//
// Build #14.
func IsWikiBatchAuditTerminalAction(a WikiBatchAuditAction) bool {
	return a == WikiBatchAuditActionFinish ||
		a == WikiBatchAuditActionUndoDone ||
		a == WikiBatchAuditActionExpire ||
		a == WikiBatchAuditActionCancel
}

// IsValidWikiBatchAuditAction is the closed-set membership check used
// by the list-by-KB handler to reject unknown action filters early.
// Returned as `bool` (not error) so handlers can collapse the check
// into a 400 response without extra wrapping.
//
// Build #14.
func IsValidWikiBatchAuditAction(a WikiBatchAuditAction) bool {
	switch a {
	case WikiBatchAuditActionEnqueue,
		WikiBatchAuditActionStart,
		WikiBatchAuditActionFinish,
		WikiBatchAuditActionUndoRequest,
		WikiBatchAuditActionUndoDone,
		WikiBatchAuditActionCancel,
		WikiBatchAuditActionExpire:
		return true
	}
	return false
}

// WikiExtractionGranularity controls how aggressive Pass 0 (candidate slug
// extraction) is. Higher granularity = more slugs, lower = tighter focus on
// the document's main subjects.
type WikiExtractionGranularity string

const (
	// WikiExtractionFocused keeps only the document's main subjects (e.g.
	// a resume yields the person + their projects, nothing else). Most
	// aggressive slug pruning; avoids index bloat from incidental technology
	// names and generic concepts.
	WikiExtractionFocused WikiExtractionGranularity = "focused"

	// WikiExtractionStandard is the default: main subjects plus entities /
	// concepts that are substantively discussed (a dedicated paragraph or
	// multiple bullet points). Skips one-off mentions and commodity terms.
	WikiExtractionStandard WikiExtractionGranularity = "standard"

	// WikiExtractionExhaustive extracts every named entity and recognizable
	// concept, including stacks/libs mentioned in passing. Matches the
	// pre-granularity behavior. Useful when the KB is being used as a
	// glossary rather than a curated wiki.
	WikiExtractionExhaustive WikiExtractionGranularity = "exhaustive"
)

// IsValid reports whether g is one of the three recognized levels.
func (g WikiExtractionGranularity) IsValid() bool {
	switch g {
	case WikiExtractionFocused, WikiExtractionStandard, WikiExtractionExhaustive:
		return true
	}
	return false
}

// Normalize returns g if valid, otherwise WikiExtractionStandard. Callers
// pipe config through this so historical rows with empty / unknown values
// don't surprise the extraction prompt.
func (g WikiExtractionGranularity) Normalize() WikiExtractionGranularity {
	if g.IsValid() {
		return g
	}
	return WikiExtractionStandard
}

// WikiConfig stores wiki-specific configuration for a knowledge base.
// Applicable to document-type knowledge bases with wiki feature enabled.
// Whether the wiki feature is turned on is controlled by IndexingStrategy.WikiEnabled;
// this struct only carries wiki-specific tunables.
type WikiConfig struct {
	// SynthesisModelID is the LLM model ID used for wiki page generation and updates
	SynthesisModelID string `yaml:"synthesis_model_id" json:"synthesis_model_id"`
	// MaxPagesPerIngest limits pages created/updated per ingest operation (0 = no limit)
	MaxPagesPerIngest int `yaml:"max_pages_per_ingest" json:"max_pages_per_ingest"`
	// ExtractionGranularity controls how many candidate slugs Pass 0 extracts
	// per document. Empty / unknown value is treated as WikiExtractionStandard.
	ExtractionGranularity WikiExtractionGranularity `yaml:"extraction_granularity" json:"extraction_granularity,omitempty"`
	// ContentInstructions controls tone, structure and emphasis for generated
	// summary/entity/index prose. Citation and merge rules remain system-owned.
	ContentInstructions string `yaml:"content_instructions,omitempty" json:"content_instructions,omitempty"`
	// ExtractionInstructions tells candidate extraction which domain concepts
	// to emphasize without replacing the stable JSON/citation protocol.
	ExtractionInstructions string `yaml:"extraction_instructions,omitempty" json:"extraction_instructions,omitempty"`

	// Wiki ingest concurrency is two-level:
	//   1. batch-level: multiple batches per KB run concurrently in the wiki
	//      worker pool, capped by IngestMaxInflight (below).
	//   2. batch-internal: within one batch, Map and Reduce fan out with
	//      errgroups sized by IngestMapParallel / IngestReduceParallel.
	// Effective peak in-flight LLM calls for one KB ≈
	//   IngestMaxInflight × max(IngestMapParallel, IngestReduceParallel),
	// further bounded by the wiki pool size (WEKNORA_WIKI_ASYNQ_CONCURRENCY).

	// IngestBatchSize controls how many pending ops a single batch claims and
	// processes before scheduling a follow-up. 0 falls back to the hard-coded
	// default (5). Larger batches amortize per-batch setup and let more docs
	// share the batch-internal Map/Reduce fan-out; smaller batches spread a
	// KB's backlog across more concurrent batches (finer scheduling grain).
	IngestBatchSize int `yaml:"ingest_batch_size" json:"ingest_batch_size,omitempty"`

	// IngestMapParallel sets the errgroup limit for the Map phase
	// (per-document extraction + summary + chunk citation) WITHIN one batch.
	// 0 falls back to 10. Bound by the LLM provider's concurrency limit and
	// the worker's outbound HTTP pool. Remember it multiplies with the number
	// of concurrent batches (IngestMaxInflight).
	IngestMapParallel int `yaml:"ingest_map_parallel" json:"ingest_map_parallel,omitempty"`

	// IngestReduceParallel sets the errgroup limit for the Reduce phase
	// (per-slug page write) WITHIN one batch. 0 falls back to 10. Bound by the
	// same LLM concurrency / HTTP pool considerations as the Map phase, plus
	// DB connection pool size. Same multiplier caveat as IngestMapParallel.
	IngestReduceParallel int `yaml:"ingest_reduce_parallel" json:"ingest_reduce_parallel,omitempty"`

	// IngestMaxInflight caps how many ingest batches for THIS KB may run
	// concurrently in the shared wiki worker pool (standard/Redis mode
	// only). 0 falls back to the hard-coded default (4). Since Phase 3
	// removed the exclusive per-KB lock, one KB's backlog could otherwise
	// monopolize the whole pool during a bulk import and starve other KBs;
	// this knob trades a single KB's peak throughput for cross-KB fairness.
	// Set it >= the wiki pool size to effectively disable the cap.
	IngestMaxInflight int `yaml:"ingest_max_inflight" json:"ingest_max_inflight,omitempty"`
}

// IngestBatchSizeOrDefault returns IngestBatchSize when set (> 0),
// otherwise the hard-coded fallback. Centralized so callers don't have
// to repeat the 0-check.
func (c *WikiConfig) IngestBatchSizeOrDefault(fallback int) int {
	if c == nil || c.IngestBatchSize <= 0 {
		return fallback
	}
	return c.IngestBatchSize
}

// IngestMapParallelOrDefault returns IngestMapParallel when set,
// otherwise the hard-coded fallback.
func (c *WikiConfig) IngestMapParallelOrDefault(fallback int) int {
	if c == nil || c.IngestMapParallel <= 0 {
		return fallback
	}
	return c.IngestMapParallel
}

// IngestReduceParallelOrDefault returns IngestReduceParallel when set,
// otherwise the hard-coded fallback.
func (c *WikiConfig) IngestReduceParallelOrDefault(fallback int) int {
	if c == nil || c.IngestReduceParallel <= 0 {
		return fallback
	}
	return c.IngestReduceParallel
}

// IngestMaxInflightOrDefault returns IngestMaxInflight when set,
// otherwise the hard-coded fallback.
func (c *WikiConfig) IngestMaxInflightOrDefault(fallback int) int {
	if c == nil || c.IngestMaxInflight <= 0 {
		return fallback
	}
	return c.IngestMaxInflight
}

// Value implements the driver.Valuer interface
func (c WikiConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface
func (c *WikiConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

// WikiPageListRequest represents a request to list wiki pages with filtering
type WikiPageListRequest struct {
	KnowledgeBaseID string      `json:"knowledge_base_id"`
	PageType        string      `json:"page_type,omitempty"`      // filter by type
	Status          string      `json:"status,omitempty"`         // filter by status
	Query           string      `json:"query,omitempty"`          // full-text search
	FolderID        *string     `json:"folder_id,omitempty"`      // exact folder placement ("" = root)
	CategoryPath    StringArray `json:"category_path,omitempty"`  // exact directory path
	CategoryDepth   *int        `json:"category_depth,omitempty"` // exact directory depth, including 0 for root
	Page            int         `json:"page,omitempty"`           // pagination page (1-based)
	PageSize        int         `json:"page_size,omitempty"`      // pagination size
	SortBy          string      `json:"sort_by,omitempty"`        // "updated_at", "created_at", "title"
	SortOrder       string      `json:"sort_order,omitempty"`     // "asc" or "desc"
}

// WikiPageListResponse represents a paginated list of wiki pages
type WikiPageListResponse struct {
	Pages      []*WikiPage `json:"pages"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// WikiGraphMode enumerates the graph query modes exposed to the API.
const (
	// WikiGraphModeOverview returns the top-N most-connected pages as an
	// overview of the knowledge base. Intended for the first graph open.
	WikiGraphModeOverview = "overview"
	// WikiGraphModeEgo returns the neighborhood around a center page up to a
	// configurable depth. Intended for drill-down interactions.
	WikiGraphModeEgo = "ego"
)

// WikiGraphRequest is the service-layer input for graph queries. It is
// populated by the HTTP handler from query params and passed down to the
// service, which is responsible for enforcing mode-specific semantics.
//
// Limit policy: a non-positive `Limit` means "no cap" and is reserved for
// internal callers (e.g. wiki lint) that need the full graph. The HTTP
// handler always clamps `Limit` into a safe range before calling the
// service so external traffic can never request an uncapped graph.
type WikiGraphRequest struct {
	KnowledgeBaseID string
	Mode            string   // "overview" (default) | "ego"
	Center          string   // ego mode center slug (required when Mode == "ego")
	Depth           int      // ego mode BFS depth, >= 1
	Types           []string // optional page_type filter; empty = no filter
	Limit           int      // max nodes to return; <= 0 means uncapped
	// FamiliarKnowledgeIDs are documents this person keeps drawing answers
	// from. Pages whose source_refs intersect the set are marked Familiar so
	// the existing Wiki graph can light them up without cloning a second graph.
	FamiliarKnowledgeIDs []string
}

// WikiGraphData represents the link graph structure for visualization.
type WikiGraphData struct {
	Nodes []WikiGraphNode `json:"nodes"`
	Edges []WikiGraphEdge `json:"edges"`
	Meta  WikiGraphMeta   `json:"meta"`
}

// WikiGraphMeta describes how the returned subgraph relates to the full
// knowledge base graph. The frontend uses `Truncated` to decide whether to
// surface a "showing X of Y" hint and to enable ego-expansion UI.
type WikiGraphMeta struct {
	Mode      string `json:"mode"`
	Total     int    `json:"total"`            // total node count in the KB before filtering/limit
	Returned  int    `json:"returned"`         // number of nodes actually returned
	Truncated bool   `json:"truncated"`        // true when Returned < Total (after filters)
	Center    string `json:"center,omitempty"` // populated in ego mode
	Depth     int    `json:"depth,omitempty"`  // populated in ego mode
	// FamiliarCount is how many returned nodes are lit up for this person.
	FamiliarCount int `json:"familiar_count,omitempty"`
}

// WikiGraphNode represents a node in the wiki link graph
type WikiGraphNode struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	PageType string `json:"page_type"`
	// Number of inbound + outbound links
	LinkCount int `json:"link_count"`
	// Familiar is true when this page was built from a document this person
	// keeps citing in answers. It is a personal overlay, not a property of
	// the page: two people looking at the same wiki see different highlights.
	Familiar bool `json:"familiar,omitempty"`
}

// WikiGraphEdge represents a directed edge in the wiki link graph
type WikiGraphEdge struct {
	Source string `json:"source"` // source slug
	Target string `json:"target"` // target slug
}

// WikiStats provides aggregate statistics about the wiki
type WikiStats struct {
	TotalPages    int64            `json:"total_pages"`
	PagesByType   map[string]int64 `json:"pages_by_type"`
	TotalLinks    int64            `json:"total_links"`
	OrphanCount   int64            `json:"orphan_count"`   // pages with no inbound links
	RecentUpdates []*WikiPage      `json:"recent_updates"` // last N updated pages
	PendingTasks  int64            `json:"pending_tasks"`  // number of documents waiting to be ingested
	PendingIssues int64            `json:"pending_issues"` // number of pending wiki issues
	IsActive      bool             `json:"is_active"`      // whether wiki ingestion is currently running
}

// WikiPageIssue represents an issue flagged on a specific wiki page.
// These issues are typically identified by agents or linters and stored for review.
type WikiPageIssue struct {
	ID                    string         `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID              uint64         `json:"tenant_id" gorm:"index"`
	KnowledgeBaseID       string         `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	Slug                  string         `json:"slug" gorm:"type:varchar(255);index"`
	IssueType             string         `json:"issue_type" gorm:"type:varchar(50)"`
	Description           string         `json:"description" gorm:"type:text"`
	SuspectedKnowledgeIDs StringArray    `json:"suspected_knowledge_ids" gorm:"type:json"`
	Status                string         `json:"status" gorm:"type:varchar(20);default:'pending';index"`
	ReportedBy            string         `json:"reported_by" gorm:"type:varchar(100)"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// TableName specifies the database table name
func (WikiPageIssue) TableName() string {
	return "wiki_page_issues"
}

// WikiIndexEntry is a single row in the structured wiki index response.
// Only the columns needed to render a clickable directory entry are
// carried — the backend projects SELECT slug, title, summary so a 40k-
// page KB does not pay for TEXT content transport on every index open.
type WikiIndexEntry struct {
	Slug         string      `json:"slug"`
	Title        string      `json:"title"`
	Summary      string      `json:"summary"`
	ParentSlug   string      `json:"parent_slug,omitempty"`
	CategoryPath StringArray `json:"category_path,omitempty"`
	WikiPath     string      `json:"wiki_path,omitempty"`
	Depth        int         `json:"depth,omitempty"`
	SortOrder    int         `json:"sort_order,omitempty"`
}

// WikiIndexGroup bundles the entries for one page_type into a page-sized
// slice. `Total` is the full count across the KB for the type; `Items`
// holds the current paginated window starting at `NextOffset - len(Items)`.
// An empty NextCursor means the window is already at the end of the type.
type WikiIndexGroup struct {
	Type       string           `json:"type"`
	Total      int64            `json:"total"`
	Items      []WikiIndexEntry `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

// WikiIndexResponse is what GET /wiki/index returns. The heavy directory
// markdown that used to sit in wiki_pages.content is gone — only the LLM-
// generated intro survives there. Everything else is assembled on demand
// from the index repo's light-column projection, keeping index reads
// O(page_size) regardless of KB size.
type WikiIndexResponse struct {
	Intro   string           `json:"intro"`
	Version int              `json:"version"`
	Groups  []WikiIndexGroup `json:"groups"`
}

// WikiPageLite is a slim projection of WikiPage carrying only the fields
// the wiki ingest pipeline reaches for during Map / Reduce. It exists so
// per-batch fetcher queries don't have to load the full multi-MB content
// column for every page they want a title or out-link from.
//
// Use cases:
//
//   - SlugTitleFetcher: resolve slug -> title for cross-link injection.
//   - cleanDeadLinks: read out_links + status without pulling content.
//   - dedup pre-filter: title + aliases + page_type for the trgm /
//     surface-similarity comparisons.
//
// Aliases is included because dedup and cross-link injection both treat
// the alias surface forms as first-class match targets; OutLinks is
// included so dead-link cleanup can determine which pages reference a
// given dead slug without a second query.
type WikiPageLite struct {
	Slug      string      `json:"slug"`
	Title     string      `json:"title"`
	PageType  string      `json:"page_type"`
	Status    string      `json:"status"`
	Aliases   StringArray `json:"aliases,omitempty"`
	OutLinks  StringArray `json:"out_links,omitempty"`
	UpdatedAt time.Time   `json:"updated_at,omitempty"`
}

// WikiPageBacklink is the resolved-row shape returned by
// `GET /wiki/pages/:slug/backlinks`. It carries only the fields
// the panel needs (slug + title + page_type + status + updated_at)
// and is intentionally a separate public type from `WikiPageLite`
// so the backlinks endpoint can evolve its payload without leaking
// the lite projection's internal fields (aliases / out_links) into
// the panel's HTTP response.
//
// Build #11.
type WikiPageBacklink struct {
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	PageType  string    `json:"page_type"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WikiBacklinkIndirect is a 2-hop backlink: a page that links to one of
// the pages that link to the current page. `Via` records the immediate
// (1-hop) slug that introduced the indirection, so the panel can show
// "(via page-a)" and the click handler can navigate to `Via` (D5) rather
// than jumping to a page two hops away from the reader's focus.
//
// Build #20.
type WikiBacklinkIndirect struct {
	*WikiPageBacklink
	Via string `json:"via"`
}

// WikiPageBacklinkRelated is a "related page" computed by Jaccard
// similarity over the current page's `out_links` set vs the candidate's
// `out_links`. The score is in [0, 1] — pages below the configured
// threshold are filtered out at the service layer; the panel uses the
// value verbatim in its chip prefix (e.g. "+0.78").
//
// Build #20.
type WikiPageBacklinkRelated struct {
	*WikiPageBacklink
	Jaccard float64 `json:"jaccard"`
}

// WikiBacklinkBroken represents a target slug in the current page's
// `out_links` that does not resolve to any existing page in the KB.
// The panel surfaces these as a read-only list with a "target was
// deleted or renamed" hint; there is no click handler (D3: per-page
// only, no full-KB lint traversal).
//
// Build #20.
type WikiBacklinkBroken struct {
	TargetSlug string `json:"target_slug"`
}

// WikiBacklinkGraphStats summarises the four sections in a single
// payload field so the panel header can render a one-line summary
// (e.g. "12 direct · 38 indirect · 5 related · 2 broken") without
// re-deriving counts client-side. OutLinkCount is the total number
// of slugs the current page points to, including broken ones.
//
// Build #20.
type WikiBacklinkGraphStats struct {
	DirectCount   int `json:"direct_count"`
	IndirectCount int `json:"indirect_count"`
	RelatedCount  int `json:"related_count"`
	BrokenCount   int `json:"broken_count"`
	OutLinkCount  int `json:"out_link_count"`
}

// WikiBacklinkGraph is the payload returned by
// `GET /wiki/pages/:slug/backlinks/graph`. It bundles four sections
// (direct / indirect / related / broken) and a stats summary so the
// panel can render the full backlinks picture in a single round-trip.
//
// Build #20.
type WikiBacklinkGraph struct {
	Direct   []*WikiPageBacklink        `json:"direct"`
	Indirect []*WikiBacklinkIndirect    `json:"indirect"`
	Related  []*WikiPageBacklinkRelated `json:"related"`
	Broken   []*WikiBacklinkBroken      `json:"broken"`
	Stats    WikiBacklinkGraphStats     `json:"stats"`
}

// WikiBacklinkGraphRequest is the service-layer input for
// `ListBacklinkGraph`. The handler clamps each numeric field to a
// safe range before invoking the service, but the service applies
// the same defaults defensively so harness tests can call it
// directly.
//
// Build #20.
type WikiBacklinkGraphRequest struct {
	KbID             string
	Slug             string
	MaxIndirect      int
	MaxRelated       int
	JaccardThreshold float64
}

// Build #21 — persisted cache for the backlinks graph payload. The four
// sections + stats are stored as raw JSON strings (TEXT in SQL) so the
// repository layer can hand them straight to GORM's column driver
// without a custom Scan/Value pair per dialect. The strings are
// canonicalised JSON (json.Marshal) so equality and indexing behave
// uniformly. source_event_id records which wiki_event produced this
// snapshot; nullable because the very first snapshot has no event yet
// (cold read → write).
type WikiBacklinksCacheRow struct {
	KbID          string `gorm:"primaryKey;column:kb_id;size:64"`
	Slug          string `gorm:"primaryKey;column:slug;size:512"`
	DirectJSON    string `gorm:"column:direct_json;type:text;not null"`
	IndirectJSON  string `gorm:"column:indirect_json;type:text;not null"`
	RelatedJSON   string `gorm:"column:related_json;type:text;not null"`
	BrokenJSON    string `gorm:"column:broken_json;type:text;not null"`
	StatsJSON     string `gorm:"column:stats_json;type:text;not null"`
	SourceEventID string `gorm:"column:source_event_id;size:64"`
	ComputedAt    time.Time `gorm:"column:computed_at;not null"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null"`
}

// TableName pins the GORM-pluralised default to the migration's
// singular name. Mirrors the pattern in WikiPageRepo (Build #11).
func (WikiBacklinksCacheRow) TableName() string {
	return "wiki_backlinks_cache"
}

// WikiBacklinksCacheStatus is the slim payload returned by
// GET /pages/:slug/backlinks/cache-status. Only the timestamps +
// source_event_id — never the full graph — so admin / debug callers
// can confirm the cache is fresh without paying the column scan cost.
//
// Build #23 adds 4 aggregate fields (kb_id, row_count, hit_ratio,
// payload_size_bytes). All new fields are populated by the handler —
// the underlying cache row never grew them. Old fields keep their
// JSON shape (omitempty on SourceEventID stays).
type WikiBacklinksCacheStatus struct {
	Slug             string    `json:"slug"`
	KbID             string    `json:"kb_id"`
	ComputedAt       time.Time `json:"computed_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	SourceEventID    string    `json:"source_event_id,omitempty"`
	RowCount         int64     `json:"row_count"`
	HitRatio         float64   `json:"hit_ratio"`
	PayloadSizeBytes int64     `json:"payload_size_bytes"`
}

// WikiBacklinksCacheStatusListResponse is the envelope returned by
// GET /knowledgebase/:kb_id/wiki/backlinks/cache-statuses. The four
// rollup fields (row_count, payload_size_bytes, hit_ratio, kb_id)
// summarise the KB's cache state, and Items is the paginated list of
// per-row statuses. Total is the unpaginated row count so the admin
// UI can render a pager.
//
// Build #23 — pure additive shape; no existing fields move or rename.
type WikiBacklinksCacheStatusListResponse struct {
	KbID             string                      `json:"kb_id"`
	RowCount         int64                       `json:"row_count"`
	PayloadSizeBytes int64                       `json:"payload_size_bytes"`
	HitRatio         float64                     `json:"hit_ratio"`
	Items            []*WikiBacklinksCacheStatus `json:"items"`
	Total            int64                       `json:"total"`
}

// WikiBacklinksCacheInvalidationLogEntry is one row in
// wiki_backlinks_cache_invalidation_log (Build #23). Inserted by
// every InvalidateBacklinksCache call (and by Build #22's sweeper
// DeleteStale). The caller computes the slug set ahead of time and
// supplies Details as a JSON-serialised map.
type WikiBacklinksCacheInvalidationLogEntry struct {
	ID            uint64    `gorm:"primaryKey;column:id;autoIncrement"`
	KbID          string    `gorm:"column:kb_id;size:64;not null"`
	Slug          string    `gorm:"column:slug;size:512;not null"`
	Op            string    `gorm:"column:op;size:32;not null"`
	ActorUserID   *uint64   `gorm:"column:actor_user_id"`
	SourceEventID string    `gorm:"column:source_event_id;size:64"`
	AffectedCount int       `gorm:"column:affected_count;not null;default:0"`
	Details       string    `gorm:"column:details;type:text"` // JSON-encoded
	CreatedAt     time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP"`
}

// TableName pins the GORM-pluralised default to the migration's
// singular name.
func (WikiBacklinksCacheInvalidationLogEntry) TableName() string {
	return "wiki_backlinks_cache_invalidation_log"
}

// BacklinkCacheInvalidateSweep is the eighth op, used by Build #22's
// stale-cleanup sweeper. We append rather than enumerate in the
// existing 7-op const block to keep the public API surface stable —
// service-layer code already switches on string equality.
const BacklinkCacheInvalidateSweep BacklinkCacheInvalidateOp = "cleanup_sweep"

// BacklinkCacheInvalidateOp enumerates the seven write paths that
// invalidate one or more (kb_id, slug) cache rows. ResolveAffectedSlugs
// (in service/wiki_backlinks_cache.go) maps each op to its slug set.
type BacklinkCacheInvalidateOp string

const (
	BacklinkCacheInvalidateCreatePage  BacklinkCacheInvalidateOp = "create_page"
	BacklinkCacheInvalidateUpdatePage  BacklinkCacheInvalidateOp = "update_page"
	BacklinkCacheInvalidateDeletePage  BacklinkCacheInvalidateOp = "delete_page"
	BacklinkCacheInvalidateMovePage    BacklinkCacheInvalidateOp = "move_page"
	BacklinkCacheInvalidateBatchMove   BacklinkCacheInvalidateOp = "batch_move"
	BacklinkCacheInvalidateBatchDelete BacklinkCacheInvalidateOp = "batch_delete"
	BacklinkCacheInvalidateBatchStatus BacklinkCacheInvalidateOp = "batch_status"
)

// BacklinkCacheInvalidateRequest is the input to
// WikiPageService.InvalidateBacklinksCache. AffectedSlugs is the slug
// set the caller has already resolved (e.g. A + A.out_links for
// UpdatePage); the service then runs the cache wipe in one DELETE
// statement with IN (?, ?, ...) batching.
type BacklinkCacheInvalidateRequest struct {
	KbID          string
	Op            BacklinkCacheInvalidateOp
	AffectedSlugs []string
}

// WikiSourceKnowledgeID extracts the knowledge id from a source_refs entry,
// stored as "uuid" or "uuid|title".
func WikiSourceKnowledgeID(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if i := strings.IndexByte(ref, '|'); i > 0 {
		return strings.TrimSpace(ref[:i])
	}
	return ref
}

// SourceKnowledgeIDs returns the document ids this page was built from.
func (p *WikiPage) SourceKnowledgeIDs() []string {
	if p == nil || len(p.SourceRefs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(p.SourceRefs))
	for _, ref := range p.SourceRefs {
		if id := WikiSourceKnowledgeID(ref); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// BuiltFrom reports whether any of this page's sources is in the given set.
func (p *WikiPage) BuiltFrom(knowledgeIDs map[string]struct{}) bool {
	if p == nil || len(knowledgeIDs) == 0 {
		return false
	}
	for _, id := range p.SourceKnowledgeIDs() {
		if _, ok := knowledgeIDs[id]; ok {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Page-level ACL (Build #7 backend)
//
// WikiPageAcl carries the per-page access control payload stored in the
// `acl` JSON column on wiki_pages. The shape mirrors the frontend contract
// in frontend/src/api/wiki/acl.ts — any change here must be paired with
// the matching change in the wikiPageAcl Pinia store + WikiAclDialog.
// -----------------------------------------------------------------------------

// WikiPageAclMode enumerates the three access modes a page can carry.
// Stored as a plain string in JSON, validated by IsValidWikiPageAclMode.
const (
	// WikiPageAclModeInherit — every KB member can read; legacy default
	// for rows where the column is NULL.
	WikiPageAclModeInherit = "inherit"
	// WikiPageAclModePrivate — only the page owner (and KB admin) can read.
	WikiPageAclModePrivate = "private"
	// WikiPageAclModeAllowList — owner + KB admin + users/groups listed in
	// AllowUserIDs / AllowGroupIDs can read.
	WikiPageAclModeAllowList = "allow_list"
)

// IsValidWikiPageAclMode reports whether mode is one of the known
// WikiPageAclMode constants. Empty is treated as inherit (legacy NULL).
func IsValidWikiPageAclMode(mode string) bool {
	switch mode {
	case "", WikiPageAclModeInherit, WikiPageAclModePrivate, WikiPageAclModeAllowList:
		return true
	}
	return false
}

// WikiPageAcl is the per-page access control record. Stored as JSON on
// wiki_pages.acl. The struct is also what the GET/PUT /api/v1/.../acl REST
// endpoints marshal to and from.
type WikiPageAcl struct {
	Mode          string   `json:"mode"`
	AllowUserIDs  []string `json:"allow_user_ids"`
	AllowGroupIDs []string `json:"allow_group_ids"`
	DenyInherited bool     `json:"deny_inherited"`
	Revision      int64    `json:"revision,omitempty"`
	UpdatedAt     string   `json:"updated_at,omitempty"`
}

// Value implements driver.Valuer so GORM can write the column as JSON.
func (c WikiPageAcl) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements sql.Scanner so GORM can read the column back into the
// struct. Mirrors WikiConfig.Scan (this file:608).
func (c *WikiPageAcl) Scan(value interface{}) error {
	if value == nil {
		*c = WikiPageAcl{Mode: WikiPageAclModeInherit}
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		*c = WikiPageAcl{Mode: WikiPageAclModeInherit}
		return nil
	}
	if len(b) == 0 {
		*c = WikiPageAcl{Mode: WikiPageAclModeInherit}
		return nil
	}
	if err := json.Unmarshal(b, c); err != nil {
		return err
	}
	if c.Mode == "" {
		c.Mode = WikiPageAclModeInherit
	}
	return nil
}

// WikiPageAclSaveRequest is what the PUT endpoint accepts. BaseRevision is
// the optimistic-lock token: when set, the server only accepts the write
// if the stored revision still equals BaseRevision. Stale writes get 409.
type WikiPageAclSaveRequest struct {
	Mode          string   `json:"mode"`
	AllowUserIDs  []string `json:"allow_user_ids"`
	AllowGroupIDs []string `json:"allow_group_ids"`
	DenyInherited bool     `json:"deny_inherited"`
	BaseRevision  int64    `json:"base_revision"`
}

// WikiPageAclDecision is the enum returned by WikiAclService.Resolve. It
// expresses the outcome of evaluating ACL for one caller against one page.
const (
	// WikiPageAclAllow — caller may read the page content.
	WikiPageAclAllow = "allow"
	// WikiPageAclDenyPrivate — page is private and caller is not owner/admin.
	// Handler maps this to HTTP 404 (not 403) to avoid leaking page existence.
	WikiPageAclDenyPrivate = "deny_private"
	// WikiPageAclDenyAllowList — page is allow_list and caller is not in
	// the allow list (nor owner/admin). Same 404 mapping as deny_private.
	WikiPageAclDenyAllowList = "deny_allow_list"
)
