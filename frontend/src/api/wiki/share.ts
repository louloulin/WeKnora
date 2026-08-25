import { get, post, del } from '../../utils/request'

/**
 * Wiki public share-link API client (Build #6).
 *
 * Endpoints (to be implemented server-side):
 *   POST   /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/shares
 *   GET    /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/shares
 *   DELETE /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/shares/:share_id
 *   GET    /api/v1/wiki/share/:token           (public — no auth)
 *   POST   /api/v1/wiki/share/:token/password  (public — password unlock)
 *
 * The authed endpoints use the wiki barrel's slug encoder; the public
 * viewer endpoint uses the raw token, which is already URL-safe.
 */

export interface WikiShareLink {
  id: string
  kbId: string
  pageSlug: string
  /** URL-safe share token (e.g. base64url(16 random bytes)). */
  token: string
  /** Absolute URL `${origin}/wiki/share/${token}`. */
  url: string
  /** Optional password lock — the UI prompts when set. */
  hasPassword: boolean
  /** ISO timestamp; null means the link never expires. */
  expiresAt: string | null
  /** ISO timestamp; once set the link returns 410 on the public endpoint. */
  revokedAt: string | null
  createdById: string
  createdByName: string
  createdAt: string
  /** Count of times this link has been opened by a public viewer. */
  viewCount: number
  lastViewedAt: string | null
}

export interface WikiShareCreateRequest {
  /** Expiry policy: '24h' | '7d' | '30d' | 'never'. */
  expiresIn: '24h' | '7d' | '30d' | 'never'
  /** Optional password to require on the public viewer. */
  password?: string
}

export interface WikiShareListResponse {
  shares: WikiShareLink[]
}

export interface WikiSharePublicResponse {
  page: {
    title: string
    summary: string
    /** Sanitized HTML ready for injection into a watermark container. */
    contentHtml: string
    pageType: string
    updatedAt: string
  }
  kb: {
    id: string
    name: string
  }
  /** Watermark text — typically the truncated token + IP hash. */
  watermark: string
}

export interface WikiSharePasswordRequest {
  password: string
}

function sharePath(kbId: string, slug: string): string {
  return `/api/v1/knowledgebase/${kbId}/wiki/pages/${slug}/shares`
}

export function listWikiShareLinks(kbId: string, slug: string) {
  return get<WikiShareListResponse>(sharePath(kbId, slug))
}

export function createWikiShareLink(
  kbId: string,
  slug: string,
  data: WikiShareCreateRequest,
) {
  return post<WikiShareLink>(sharePath(kbId, slug), data)
}

export function revokeWikiShareLink(kbId: string, slug: string, shareId: string) {
  return del<void>(`${sharePath(kbId, slug)}/${encodeURIComponent(shareId)}`)
}

export function fetchPublicShare(token: string) {
  return get<WikiSharePublicResponse>(`/api/v1/wiki/share/${encodeURIComponent(token)}`)
}

export function unlockPublicShare(token: string, password: string) {
  return post<WikiSharePublicResponse>(
    `/api/v1/wiki/share/${encodeURIComponent(token)}/password`,
    { password } satisfies WikiSharePasswordRequest,
  )
}