import { get, post, put, del } from '../../utils/request'

/** Mirror of api/wiki/index.ts#encodeSlugPath — kept local so this client
 *  doesn't add a new export to the wiki barrel and stays drop-in. */
function encodeSlugPath(slug: string): string {
  return slug.split('/').map(encodeURIComponent).join('/')
}

/**
 * Wiki page comment + @mention API client (Build #5).
 *
 * Endpoints (to be implemented server-side alongside this client):
 *   GET    /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/comments
 *   POST   /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/comments
 *   PUT    /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/comments/:comment_id
 *   DELETE /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/comments/:comment_id
 *   GET    /api/v1/knowledgebase/:kb_id/wiki/mentions/search?q=:query
 *
 * The frontend treats these endpoints as the contract — until the
 * backend lands, list calls return `{ comments: [] }` and create /
 * update / delete are best-effort. The store fails-open on 404 so
 * the drawer renders gracefully.
 */

export interface WikiCommentMention {
  /** Stable user id from the identity provider; the @mention reference. */
  userId: string
  /** Display name written into the comment body as @Name. */
  displayName: string
  /** Optional handle / username (without @); falls back to displayName. */
  handle?: string
  /** Avatar URL if available. */
  avatarUrl?: string
}

export interface WikiComment {
  id: string
  tenantId: number
  knowledgeBaseId: string
  pageSlug: string
  parentId?: string
  /** Markdown or plain-text body (max 4 KB). */
  body: string
  /** Mentioned users referenced in `body`. */
  mentions: WikiCommentMention[]
  authorId: string
  authorName: string
  authorAvatarUrl?: string
  /** ISO timestamp; comments are append-only after creation. */
  createdAt: string
  updatedAt: string
  /** Resolved marker; null when the thread is open. */
  resolvedAt?: string | null
}

export interface WikiCommentListResponse {
  comments: WikiComment[]
}

export interface WikiCommentCreateRequest {
  body: string
  parentId?: string
  mentions?: WikiCommentMention[]
}

export interface WikiCommentUpdateRequest {
  body: string
  mentions?: WikiCommentMention[]
}

export interface WikiMentionCandidate {
  userId: string
  displayName: string
  handle?: string
  avatarUrl?: string
}

export interface WikiMentionSearchResponse {
  candidates: WikiMentionCandidate[]
}

function commentsPath(kbId: string, slug: string): string {
  return `/api/v1/knowledgebase/${kbId}/wiki/pages/${encodeSlugPath(slug)}/comments`
}

export function listWikiComments(kbId: string, slug: string) {
  return get<WikiCommentListResponse>(commentsPath(kbId, slug))
}

export function createWikiComment(kbId: string, slug: string, data: WikiCommentCreateRequest) {
  return post<WikiComment>(commentsPath(kbId, slug), data)
}

export function updateWikiComment(
  kbId: string,
  slug: string,
  commentId: string,
  data: WikiCommentUpdateRequest,
) {
  return put<WikiComment>(`${commentsPath(kbId, slug)}/${encodeURIComponent(commentId)}`, data)
}

export function deleteWikiComment(kbId: string, slug: string, commentId: string) {
  return del<void>(`${commentsPath(kbId, slug)}/${encodeURIComponent(commentId)}`)
}

export function searchMentionCandidates(kbId: string, query: string, limit = 8) {
  const qs = new URLSearchParams({ q: query, limit: String(limit) }).toString()
  return get<WikiMentionSearchResponse>(
    `/api/v1/knowledgebase/${kbId}/wiki/mentions/search?${qs}`,
  )
}