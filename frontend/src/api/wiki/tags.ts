import { get, post, put, del } from '../../utils/request'
import type {
  WikiBatchTagBody,
  WikiTag,
  WikiTagCreateRequest,
  WikiTagListResponse,
  WikiTagUpdateRequest,
  WikiPageTagsResponse,
  WikiTagWithCount,
} from './tagTypes'
import type { WikiBatchRouteResult } from './batchTypes'

export {
  type WikiBatchTagBody,
  type WikiTag,
  type WikiTagColor,
  type WikiTagCreateRequest,
  type WikiTagListResponse,
  type WikiTagUpdateRequest,
  type WikiPageTagsResponse,
  type WikiTagWithCount,
  type WikiTagSetPageRequest,
  WikiBatchTagOpAdd,
  WikiBatchTagOpRemove,
  WikiTagPalette,
} from './tagTypes'

export type { WikiBatchRouteResult }

/**
 * Wiki tag API client (Build #17 / P1.1).
 *
 * Endpoints (8) — all under
 * `/api/v1/knowledgebase/:kb_id/wiki/...`:
 *
 *   GET    /tags                          list tag definitions
 *   POST   /tags                          create one
 *   GET    /tags/:tag_id                  read one
 *   PUT    /tags/:tag_id                  partial update
 *   DELETE /tags/:tag_id                  remove definition + cascade joins
 *   GET    /pages/:slug/tags              list tags attached to a page
 *   PUT    /pages/:slug/tags              replace page-tag set atomically
 *   POST   /pages/batch-tag               add/remove tag from many pages
 *
 * The batch-tag endpoint mirrors the existing batch-* pattern: small
 * input is synchronous (returns per-row result), large input is
 * queued (returns job envelope). The UI does not need to know which
 * path was taken — both come back through the same route result.
 */

// WikiBatchRouteResult is imported from batchTypes so the store /
// components see one discriminated union across every batch-* endpoint
// instead of three near-identical duplicates.

function tagsPath(kbId: string): string {
  return `/api/v1/knowledgebase/${encodeURIComponent(kbId)}/wiki/tags`
}

function tagPath(kbId: string, tagId: string): string {
  return `${tagsPath(kbId)}/${encodeURIComponent(tagId)}`
}

// encodeSlugPath is duplicated here rather than imported to keep the
// barrel self-contained; the page-side variants already do the same.
function encodeSlugPath(slug: string): string {
  return slug.split('/').map(encodeURIComponent).join('/')
}

function pageTagsPath(kbId: string, slug: string): string {
  return `/api/v1/knowledgebase/${encodeURIComponent(kbId)}/wiki/pages/${encodeSlugPath(slug)}/tags`
}

function batchTagPath(kbId: string): string {
  return `/api/v1/knowledgebase/${encodeURIComponent(kbId)}/wiki/pages/batch-tag`
}

export function listWikiTags(kbId: string) {
  return get<WikiTagListResponse>(tagsPath(kbId))
}

export function createWikiTag(kbId: string, body: WikiTagCreateRequest) {
  return post<WikiTag>(tagsPath(kbId), body)
}

export function getWikiTag(kbId: string, tagId: string) {
  return get<WikiTag>(tagPath(kbId, tagId))
}

export function updateWikiTag(
  kbId: string,
  tagId: string,
  body: WikiTagUpdateRequest,
) {
  return put<WikiTag>(tagPath(kbId, tagId), body)
}

export function deleteWikiTag(kbId: string, tagId: string) {
  return del<void>(tagPath(kbId, tagId))
}

export function getWikiPageTags(kbId: string, slug: string) {
  return get<WikiPageTagsResponse>(pageTagsPath(kbId, slug))
}

export function setWikiPageTags(
  kbId: string,
  slug: string,
  tagIds: string[],
) {
  return put<WikiPageTagsResponse>(pageTagsPath(kbId, slug), {
    tag_ids: tagIds,
  })
}

export function batchTagWikiPages(
  kbId: string,
  body: WikiBatchTagBody,
) {
  return post<WikiBatchRouteResult>(batchTagPath(kbId), body)
}