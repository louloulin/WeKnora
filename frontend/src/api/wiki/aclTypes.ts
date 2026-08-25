/**
 * Shared types + defaults for the wiki page ACL — extracted so the
 * store helper module can be unit-tested in Node without pulling in
 * `utils/request.ts` (which transitively imports `vue-i18n`).
 *
 * `api/wiki/acl.ts` re-exports these for HTTP consumers.
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

export function defaultWikiPageAcl(): WikiPageAcl {
  return {
    mode: 'inherit',
    allowUserIds: [],
    allowGroupIds: [],
    denyInherited: false,
  }
}