import { get, put } from '../../utils/request'
import type {
  WikiAclSaveRequest,
  WikiPageAcl,
  WikiUserSearchResponse,
} from './aclTypes'

// Re-export so existing consumers (`import { WikiPageAcl } from
// 'src/api/wiki/acl'`) keep working unchanged.
export {
  type WikiAclMode,
  type WikiAclSaveRequest,
  type WikiPageAcl,
  type WikiUserCandidate,
  type WikiUserSearchResponse,
  defaultWikiPageAcl,
} from './aclTypes'

/**
 * Wiki page-level ACL API client (Build #7).
 *
 * The ACL is stored as a JSONB column on `wiki_pages`. The frontend
 * reads it on every page open, falls back to a default `inherit`
 * record when the column is missing / empty, and pushes a complete
 * record on save (PUT semantics — no partial PATCH).
 *
 * Endpoints:
 *   GET /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/acl
 *   PUT /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/acl
 *
 * The user-search endpoint is intentionally outside the wiki barrel —
 * ACL candidates are tenant-wide, not KB-scoped.
 *   GET /api/v1/users/search?q=:query&limit=:n
 */

function aclPath(kbId: string, slug: string): string {
  return `/api/v1/knowledgebase/${encodeURIComponent(kbId)}/wiki/pages/${slug}/acl`
}

export function getWikiPageAcl(kbId: string, slug: string) {
  return get<WikiPageAcl>(aclPath(kbId, slug))
}

export function putWikiPageAcl(kbId: string, slug: string, data: WikiAclSaveRequest) {
  return put<WikiPageAcl>(aclPath(kbId, slug), data)
}

export function searchWikiAclCandidates(query: string, limit = 10) {
  const qs = new URLSearchParams({ q: query, limit: String(limit) }).toString()
  return get<WikiUserSearchResponse>(`/api/v1/users/search?${qs}`)
}