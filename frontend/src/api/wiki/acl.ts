import { get, put } from '../../utils/request'

/**
 * Wiki page-level ACL API client (Build #7).
 *
 * The ACL is stored as a JSONB column on `wiki_pages`. The frontend
 * reads it on every page open, falls back to a default `inherit`
 * record when the column is missing / empty, and pushes a complete
 * record on save (PUT semantics — no partial PATCH).
 *
 * Endpoints (to be implemented server-side):
 *   GET /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/acl
 *   PUT /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/acl
 *
 * The user-search endpoint is intentionally outside the wiki barrel —
 * ACL candidates are tenant-wide, not KB-scoped.
 *   GET /api/v1/users/search?q=:query&limit=:n
 */

/**
 * ACL resolution modes:
 *   - inherit:   page inherits KB-level access; no per-page restriction.
 *   - private:   only the page owner and tenant admins can read; everyone
 *                else is denied regardless of KB membership.
 *   - allow_list: page inherits KB-level access, but only the users /
 *                groups on `allowUserIds` / `allowGroupIds` are admitted
 *                on top of KB membership.
 */
export type WikiAclMode = 'inherit' | 'private' | 'allow_list'

export interface WikiPageAcl {
  mode: WikiAclMode
  allowUserIds: string[]
  allowGroupIds: string[]
  /**
   * When true, the page is hidden from KB members who are NOT explicitly
   * on the allow list. When false, KB members can still see the page
   * metadata but content is gated.
   */
  denyInherited: boolean
  /** Server-stamped; used for optimistic-lock on save. */
  revision?: number
  updatedAt?: string
}

export interface WikiAclSaveRequest {
  mode: WikiAclMode
  allowUserIds: string[]
  allowGroupIds: string[]
  denyInherited: boolean
  /** Expected server revision; 409 → reload + reapply. */
  baseRevision?: number
}

export interface WikiUserCandidate {
  userId: string
  displayName: string
  handle?: string
  email?: string
  avatarUrl?: string
}

export interface WikiUserSearchResponse {
  candidates: WikiUserCandidate[]
}

const DEFAULT_ACL: WikiPageAcl = {
  mode: 'inherit',
  allowUserIds: [],
  allowGroupIds: [],
  denyInherited: false,
}

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

export function defaultWikiPageAcl(): WikiPageAcl {
  return { ...DEFAULT_ACL, allowUserIds: [], allowGroupIds: [] }
}